package ingestion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
)

type demuxClaim struct {
	Unit        Unit
	Receipt     Receipt
	Decision    Decision
	Method      i.Identification
	Input       object
	ExecutionID string
	Fence       int64
	Attempt     int
}

func unitTx(ctx context.Context, tx pgx.Tx, id string) (Unit, error) {
	return read[Unit](tx.QueryRow(ctx, `SELECT to_jsonb(u) FROM ingestion_units u WHERE id=$1`, id))
}
func decisionTx(ctx context.Context, tx pgx.Tx, id string) (Decision, error) {
	return read[Decision](tx.QueryRow(ctx, `SELECT `+decisionJSON+` FROM ingestion_identity_decisions d WHERE id=$1`, id))
}
func (s *Service) PlanOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	partition := shard{0, 1}
	if v, ok := ctx.Value(workerSlot{}).(shard); ok {
		partition = v
	}
	var id, tenantID string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT u.id::text,r.tenant_id::text FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id JOIN ingestion_sources s ON s.id=r.source_id JOIN ingestion_identity_decisions di ON di.id=u.decision_id WHERE ((hashtextextended(u.id::text,0) & 2147483647) % $1)=$2 AND (NOT s.planning_paused OR di.recovery_id IS NOT NULL) AND NOT s.blocked AND (u.state='identified' AND u.available_at<=clock_timestamp() OR u.state='demultiplexing' AND u.lease_until<clock_timestamp() OR u.state='demultiplex_retry' AND u.available_at<=clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM ingestion_change_intents c WHERE c.source_id=s.id AND c.state='pending') ORDER BY u.created_at LIMIT 1`, partition.count, partition.slot).Scan(&id, &tenantID)
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var demux *demuxClaim
	e = s.machineTx(ctx, tenantID, false, func(tx pgx.Tx) error {
		var live bool
		if e := tx.QueryRow(ctx, `SELECT state='identified' OR state='demultiplexing' AND lease_until<clock_timestamp() OR state='demultiplex_retry' AND available_at<=clock_timestamp() FROM ingestion_units WHERE id=$1 FOR UPDATE`, id).Scan(&live); e != nil || !live {
			return e
		}
		u, e := unitTx(ctx, tx, id)
		if e != nil {
			return e
		}
		receipt, e := receiptTx(ctx, tx, u.ReceiptID)
		if e != nil {
			return e
		}
		decision, e := decisionTx(ctx, tx, u.DecisionID)
		if e != nil {
			return e
		}
		fail := func(state string, err error) error {
			_, e := tx.Exec(ctx, `UPDATE ingestion_units SET state=$2,error_code=$3,lease_until=NULL WHERE id=$1`, id, state, code(err))
			return e
		}
		if e = s.identityAllowed(ctx, tx, receipt, decision); e != nil {
			return fail("blocked", e)
		}
		recovery, e := recoveryConfig(ctx, tx, decision)
		if e != nil {
			return e
		}
		if len(recovery.UnitIDs) > 0 {
			selected := false
			for _, targetID := range recovery.UnitIDs {
				target, e := unitTx(ctx, tx, targetID)
				if e != nil {
					return e
				}
				if target.ReceiptID == receipt.ID && (target.Key == u.Key || strings.HasPrefix(target.Key, u.Key+"/")) {
					selected = true
				}
			}
			if !selected {
				return fail("not_selected", nil)
			}
		}
		var explicit *Association
		if recovery.AssociationID != "" {
			target, e := associationTx(ctx, tx, tenantID, recovery.AssociationID)
			if e != nil {
				return e
			}
			explicit = &target
		}
		t, e := read[Terminal](tx.QueryRow(ctx, `SELECT `+terminalJSON+` FROM ingestion_terminals t WHERE source_id=$1 AND terminal_key=$2 AND tenant_id=$3`, receipt.SourceID, u.TerminalKey, tenantID))
		if e != nil {
			return fail("awaiting_identity", c.Fail("terminal_unregistered"))
		}
		if explicit != nil && (explicit.TerminalID != t.ID || u.ChildKey != "" && u.ChildKey != explicit.ChildKey) {
			return fail("not_selected", nil)
		}
		if !t.Enabled {
			return fail("blocked", c.Fail("terminal_disabled"))
		}
		// Historical intervals retain the terminal mode and demultiplexer they adopted.
		var historicalMode *string
		if e = tx.QueryRow(ctx, `SELECT CASE WHEN bool_or(demultiplexer IS NOT NULL) THEN 'multiple' WHEN count(*)>0 THEN 'single' END FROM ingestion_device_associations WHERE terminal_id=$1 AND epoch=$2 AND start_sequence<=$3 AND (end_sequence IS NULL OR end_sequence>$3)`, t.ID, receipt.Epoch, receipt.SourceSequence).Scan(&historicalMode); e != nil {
			return e
		}
		if historicalMode != nil {
			t.Mode = *historicalMode
		}
		if explicit != nil {
			if explicit.Demultiplexer != nil {
				t.Mode = "multiple"
			} else {
				t.Mode = "single"
			}
		}
		if t.Mode == "multiple" && u.ChildKey == "" && u.MessageKind == "measurement" {
			rows, e := tx.Query(ctx, `SELECT demultiplexer FROM ingestion_device_associations WHERE terminal_id=$1 AND epoch=$2 AND start_sequence<=$3 AND (end_sequence IS NULL OR end_sequence>$3) AND enabled`, t.ID, receipt.Epoch, receipt.SourceSequence)
			if e != nil {
				return e
			}
			var ref *i.Method
			ambiguous := false
			for rows.Next() {
				var raw []byte
				if e = rows.Scan(&raw); e != nil {
					rows.Close()
					return e
				}
				var v i.Method
				if e = json.Unmarshal(raw, &v); e != nil {
					rows.Close()
					return e
				}
				if ref != nil && *ref != v {
					ambiguous = true
				}
				ref = &v
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			if ref == nil && explicit != nil {
				ref = explicit.Demultiplexer
			}
			if ref == nil {
				return fail("awaiting_association", c.Fail("association_missing"))
			}
			if ambiguous {
				return fail("awaiting_identity", c.Fail("identity_ambiguous"))
			}
			method, e := s.parser.IdentificationTx(ctx, tx, *ref, false)
			if e != nil {
				return fail("blocked", e)
			}
			obj, e := objectTx(ctx, tx, receipt.ObjectID)
			if e != nil {
				return e
			}
			allowed, err := identitySlotTx(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if !allowed {
				return nil
			}
			v := demuxClaim{Unit: u, Receipt: receipt, Decision: decision, Method: method, Input: obj, ExecutionID: uuid.NewString()}
			if e = tx.QueryRow(ctx, `UPDATE ingestion_units SET state='demultiplexing',fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+interval '45 seconds' WHERE id=$1 AND attempts<5 RETURNING fence,attempts`, id).Scan(&v.Fence, &v.Attempt); e == pgx.ErrNoRows {
				return fail("rejected", c.Fail("attempts_exhausted"))
			} else if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_attempts(execution_id,stage,target_id,fence,attempt) VALUES($1,'demultiplex',$2,$3,$4)`, v.ExecutionID, id, v.Fence, v.Attempt); e != nil {
				return e
			}
			demux = &v
			return nil
		}
		association, e := read[Association](tx.QueryRow(ctx, `SELECT `+associationJSON+` FROM ingestion_device_associations a WHERE a.terminal_id=$1 AND a.child_key=$2 AND a.epoch=$3 AND a.start_sequence<=$4 AND (a.end_sequence IS NULL OR a.end_sequence>$4) AND a.enabled`, t.ID, u.ChildKey, receipt.Epoch, receipt.SourceSequence))
		if e != nil && explicit != nil && explicit.ChildKey == u.ChildKey {
			association = *explicit
			e = nil
		}
		if e != nil {
			return fail("awaiting_association", c.Fail("association_missing"))
		}
		src, e := sourceTx(ctx, tx, k.Tenant(tenantID), receipt.SourceID)
		if e != nil {
			return e
		}
		projectGrant, e := businessTx(ctx, tx, src, association.DeviceID)
		if e != nil {
			return fail("quarantined", e)
		}
		assignment := map[string]any{"source_id": src.ID, "source_project_id": src.ProjectID, "terminal_id": t.ID, "association_id": association.ID, "device_id": association.DeviceID, "source_sequence": receipt.SourceSequence, "epoch": receipt.Epoch, "project_device_grant_id": projectGrant}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_units SET tenant_id=$2,terminal_id=$3,device_id=$4,association_id=$5,assignment=$6 WHERE id=$1`, id, tenantID, t.ID, association.DeviceID, association.ID, k.JSON(assignment)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_unit_assignments(unit_id,snapshot) VALUES($1,$2)`, id, k.JSON(assignment)); e != nil {
			return e
		}
		if u.MessageKind == "control" {
			_, e = tx.Exec(ctx, `UPDATE ingestion_units SET state='control',error_code='' WHERE id=$1`, id)
			return e
		}
		grant, e := grantTx(ctx, tx, src.ID, t.ID)
		if e != nil || !grant.Enabled || !grant.Measure {
			return fail("blocked", c.Fail("execution_unconfigured"))
		}
		sourceGrant, e := grantTx(ctx, tx, src.ID, "")
		if e != nil {
			return e
		}
		frozen, e := s.parser.ProductionSnapshotTx(ctx, tx, tenantID, association.DeviceID, recovery.ConfigRevisionID)
		if e != nil {
			return fail("awaiting_configuration", e)
		}
		rev, e := revisionTx(ctx, tx, src.ID, receipt.SourceRevision)
		if e != nil {
			return e
		}
		protocol, e := s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false)
		if e != nil {
			return fail("blocked", e)
		}
		compat, e := s.parser.CompatibilityTx(ctx, tx, protocol, association.Demultiplexer, u.View.InputUnit, frozen.Snapshot.Method, false)
		if e != nil {
			return fail("awaiting_configuration", e)
		}
		plan := PlanSnapshot{ReceiptID: receipt.ID, Source: rev.Snapshot, Terminal: t, Association: association, Grant: grant, SourceGrant: sourceGrant, Compatibility: compat, Parsing: frozen, Input: u.View, ReceivedAt: receipt.ReceivedAt, TraceID: receipt.ID, ProjectGrantID: projectGrant}
		if decision.RecoveryID != nil {
			plan.RecoveryID = *decision.RecoveryID
			plan.RecoveryGeneration = *decision.RecoveryGeneration
		}
		var queued int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM ingestion_parse_plans WHERE tenant_id=$1 AND snapshot->'execution_grant'->>'id'=$2 AND state IN ('queued','parsing','retry_wait')`, tenantID, grant.ID).Scan(&queued); e != nil {
			return e
		}
		if queued >= grant.Budget {
			_, e := tx.Exec(ctx, `UPDATE ingestion_units SET available_at=clock_timestamp()+interval '30 seconds',error_code='queue_budget_exceeded' WHERE id=$1`, id)
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_parse_plans(tenant_id,unit_id,decision_id,device_id,adoption_id,snapshot) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(unit_id,decision_id) DO NOTHING`, tenantID, id, decision.ID, association.DeviceID, frozen.AdoptionID, k.JSON(plan)); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE ingestion_units SET state='planned',error_code='' WHERE id=$1`, id)
		return e
	})
	if e != nil {
		return true, e
	}
	if demux != nil {
		return true, s.demultiplex(ctx, *demux)
	}
	return true, nil
}
func (s *Service) demultiplex(ctx context.Context, job demuxClaim) error {
	raw, callErr := s.readObject(ctx, job.Input)
	if callErr == nil {
		raw, callErr = i.Materialize(raw, job.Unit.View)
	}
	if callErr == nil {
		callErr = s.machineTx(ctx, *job.Receipt.TenantID, false, func(tx pgx.Tx) error {
			if e := s.identityAllowed(ctx, tx, job.Receipt, job.Decision); e != nil {
				return e
			}
			_, e := s.parser.IdentificationTx(ctx, tx, job.Method.Method, false)
			return e
		})
	}
	var response i.Response
	var body []byte
	if callErr == nil {
		req := i.Request{Version: 1, ExecutionID: job.ExecutionID, Method: job.Method.Method, Input: i.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), SHA256: c.Digest(raw), ByteLength: int64(len(raw))}, Context: i.Context{ReceivedAt: job.Receipt.ReceivedAt, Transport: job.Method.Transport, InputUnit: job.Method.InputUnit}, TraceID: job.Receipt.ID}
		body, response, callErr = s.parser.Identify(ctx, req, job.Method)
		observeDependency(ctx, "parser", callErr)
	}
	var stored *object
	if len(body) > 0 {
		obj, e := s.saveObject(ctx, body)
		if e != nil {
			callErr = e
		} else {
			stored = &obj
		}
	}
	return s.machineTx(ctx, *job.Receipt.TenantID, false, func(tx pgx.Tx) error {
		var objectID any
		if stored != nil {
			objectID = stored.ID
		}
		state := "succeeded"
		if callErr != nil {
			state = "failed"
		}
		if _, e := tx.Exec(ctx, `UPDATE ingestion_attempts SET state=$2,error_code=$3,response_object_id=$4,finished_at=clock_timestamp() WHERE execution_id=$1`, job.ExecutionID, state, code(callErr), objectID); e != nil {
			return e
		}
		var live bool
		if e := tx.QueryRow(ctx, `SELECT state='demultiplexing' AND fence=$2 AND lease_until>clock_timestamp() FROM ingestion_units WHERE id=$1 FOR UPDATE`, job.Unit.ID, job.Fence).Scan(&live); e != nil || !live {
			return e
		}
		if e := s.identityAllowed(ctx, tx, job.Receipt, job.Decision); e != nil {
			callErr = e
		}
		if callErr == nil && response.Status != "identified" {
			callErr = c.Fail("demultiplex_rejected")
		}
		if callErr != nil {
			state := "rejected"
			if parsing.Retryable(callErr) && job.Attempt < 5 {
				state = "demultiplex_retry"
			}
			_, e := tx.Exec(ctx, `UPDATE ingestion_units SET state=$2,error_code=$3,lease_until=NULL,available_at=clock_timestamp()+$4*interval '1 second' WHERE id=$1`, job.Unit.ID, state, code(callErr), retryDelay(job.Attempt))
			return e
		}
		for _, u := range response.Units {
			if u.TerminalKey != job.Unit.TerminalKey {
				return c.Fail("parsed_identity_mismatch")
			}
			view := u.View
			view.Transforms = append(append([]i.Transform{}, job.Unit.View.Transforms...), u.View.Transforms...)
			view.Kind = "decoded"
			if job.Unit.View.Kind == "whole" {
				view.Kind = "range"
			}
			if _, e := tx.Exec(ctx, `INSERT INTO ingestion_units(receipt_id,decision_id,unit_key,terminal_key,child_key,view,message_kind,parent_unit_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, job.Receipt.ID, job.Decision.ID, job.Unit.Key+"/"+u.Key, u.TerminalKey, u.ChildKey, k.JSON(view), u.MessageKind, job.Unit.ID); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `INSERT INTO ingestion_demultiplex_decisions(unit_id,execution_id,method,response_object_id,response) VALUES($1,$2,$3,$4,$5)`, job.Unit.ID, job.ExecutionID, k.JSON(job.Method), objectID, k.JSON(response)); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE ingestion_units SET state='envelope',lease_until=NULL WHERE id=$1`, job.Unit.ID)
		return e
	})
}
