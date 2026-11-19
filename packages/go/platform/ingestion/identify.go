package ingestion

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

//go:embed candidates/identify.sql
var identifyCandidates string

type identifyClaim struct {
	Receipt     Receipt
	Decision    Decision
	Input       object
	ExecutionID string
	MachineID   string
	Source      r.Source
}

type identityAuthority struct {
	source   Source
	revision SourceRevision
	grant    Grant
	protocol i.Protocol
	method   i.Identification
}

func (s *Service) identityAllowed(ctx context.Context, tx pgx.Tx, receipt Receipt, d Decision) error {
	_, err := s.identityAuthority(ctx, tx, receipt, d)
	return err
}

// The machine transaction already holds the catalog and tenant mutation locks.
// Resolve the same source, active business scope, confirmed source revision,
// pending association change and grant from one current statement snapshot.
func (s *Service) identityAuthority(ctx context.Context, tx pgx.Tx, receipt Receipt, d Decision) (identityAuthority, error) {
	var state identityAuthority
	if d.RecoveryID != nil {
		if d.RecoveryGeneration == nil {
			return state, c.Fail("recovery_not_active")
		}
		if err := s.recoveryAllowedTx(ctx, tx, *d.RecoveryID, *d.RecoveryGeneration); err != nil {
			return state, err
		}
	}
	if receipt.TenantID == nil {
		return state, c.Fail("source_revision_unknown")
	}
	var revisionJSONBytes, grantJSONBytes []byte
	var active, changing bool
	err := tx.QueryRow(ctx, `SELECT `+sourceJSON+`,
		 `+activeRoutingProject+`,
		EXISTS(SELECT 1 FROM ingestion_change_intents c WHERE c.source_id=s.id AND c.state='pending'),
		CASE WHEN r.source_id IS NOT NULL THEN `+revisionJSON+` END,
		CASE WHEN g.id IS NOT NULL THEN `+grantJSON+` END
		FROM ingestion_sources s LEFT JOIN ingestion_source_revisions r ON r.source_id=s.id AND r.revision=$3
		LEFT JOIN ingestion_execution_grants g ON g.source_id=s.id AND g.terminal_id IS NULL
		WHERE s.id=$1 AND s.tenant_id=$2`, receipt.SourceID, *receipt.TenantID, receipt.SourceRevision).
		Scan(&state.source, &active, &changing, &revisionJSONBytes, &grantJSONBytes)
	if err == pgx.ErrNoRows {
		return state, k.NotFound("ingestion")
	}
	if err != nil {
		return state, err
	}
	if state.source.Blocked || state.source.PlanningPaused && d.RecoveryID == nil {
		return state, c.Fail("execution_paused")
	}
	if !active {
		return state, k.Problem("routing_scope_mismatch", "租户或来源项目当前不可用。", 409)
	}
	if changing {
		return state, c.Fail("association_changing")
	}
	if revisionJSONBytes == nil {
		return state, k.NotFound("ingestion")
	}
	if err = json.Unmarshal(revisionJSONBytes, &state.revision); err != nil {
		return state, err
	}
	if !state.revision.Confirmed {
		return state, c.Fail("awaiting_source_confirmation")
	}
	state.protocol, state.method, err = s.parser.IngressIdentificationTx(ctx, tx, state.revision.Snapshot.ProtocolID, state.revision.Snapshot.ProtocolVersion, false)
	if err != nil {
		return state, err
	}
	if grantJSONBytes == nil {
		return state, c.Fail("execution_unconfigured")
	}
	if err = json.Unmarshal(grantJSONBytes, &state.grant); err != nil {
		return state, c.Fail("execution_unconfigured")
	}
	grant := state.grant
	if !grant.Enabled || !grant.Identify || d.GrantID != nil && (*d.GrantID != grant.ID || d.GrantGeneration == nil || *d.GrantGeneration != grant.Generation) {
		return state, c.Fail("authorization_revoked")
	}
	return state, nil
}
func (s *Service) claimIdentity(ctx context.Context, key *workKey) (*identifyClaim, error) {
	var candidates []Receipt
	var e error
	if key != nil {
		candidates = []Receipt{{ID: key.id, TenantID: &key.scope}}
	} else {
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			unrecorded := s.unrecorded("ingest-custody-worker", "ingestion.receipt.stored", "identify", "r.id", "r.current_generation")
			query := strings.ReplaceAll(identifyCandidates, "/* split event ownership */", unrecorded)
			s.recordCandidateScan()
			rows, e := tx.Query(ctx, query)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var v Receipt
				if e := rows.Scan(&v.ID, &v.TenantID); e != nil {
					return e
				}
				candidates = append(candidates, v)
			}
			return rows.Err()
		})
		if e != nil {
			return nil, e
		}
	}
	for _, candidate := range candidates {
		var picked *identifyClaim
		e = s.machineTx(ctx, *candidate.TenantID, false, func(tx pgx.Tx) error {
			// Lock first, then take a fresh statement snapshot for the decision.
			// A correlated EXISTS evaluated in SELECT ... FOR UPDATE can retain
			// a pre-wait decision snapshot after another claimant changes its lease.
			if key != nil {
				var eligible bool
				err := tx.QueryRow(ctx, `SELECT true FROM ingestion_receipts r JOIN ingestion_sources s ON s.id=r.source_id
			    LEFT JOIN ingestion_identity_decisions d ON d.receipt_id=r.id AND d.generation=r.current_generation
			    WHERE r.id=$1 AND r.tenant_id=$2 AND r.current_generation=$3 AND NOT s.blocked AND (NOT s.planning_paused OR d.recovery_id IS NOT NULL)
			    AND NOT EXISTS(SELECT 1 FROM ingestion_change_intents c WHERE c.source_id=s.id AND c.state='pending')
			    AND (r.state='stored' AND d.id IS NULL OR r.state IN ('stored','identifying','retry_wait') AND
			        (d.state IN ('queued','retry_wait') AND d.available_at<=clock_timestamp() OR d.state='identifying' AND d.lease_until<clock_timestamp())) FOR UPDATE OF r`, key.id, key.scope, key.generation).Scan(&eligible)
				if err == pgx.ErrNoRows {
					return nil
				}
				if err != nil {
					return err
				}
			} else if _, e := tx.Exec(ctx, `SELECT id FROM ingestion_receipts WHERE id=$1 FOR UPDATE`, candidate.ID); e != nil {
				return e
			}

			var receipt Receipt
			var d Decision
			var live bool
			var decisionBytes []byte
			e := tx.QueryRow(ctx, `SELECT `+receiptJSON+`,r.state='stored' OR COALESCE(
				d.state IN ('queued','retry_wait') AND d.available_at<=clock_timestamp() OR
				d.state='identifying' AND d.lease_until<clock_timestamp(),false),
				CASE WHEN d.id IS NOT NULL THEN `+decisionJSON+` END
				FROM ingestion_receipts r LEFT JOIN ingestion_identity_decisions d ON d.receipt_id=r.id AND d.generation=r.current_generation
				WHERE r.id=$1`, candidate.ID).Scan(&receipt, &live, &decisionBytes)
			if e != nil {
				return e
			}
			if !live || key != nil && receipt.Generation != numstr(key.generation) {
				return nil
			}
			if decisionBytes != nil {
				if e = json.Unmarshal(decisionBytes, &d); e != nil {
					return e
				}
			}
			authority, e := s.identityAuthority(ctx, tx, receipt, d)
			if e != nil {
				_, err := tx.Exec(ctx, `UPDATE ingestion_receipts SET state='blocked',error_code=$2 WHERE id=$1`, receipt.ID, code(e))
				return err
			}
			allowed, err := identitySlotTx(ctx, tx, *receipt.TenantID)
			if err != nil {
				return err
			}
			if !allowed {
				return nil
			}

			if d.Attempts >= 5 {
				_, e = tx.Exec(ctx, `UPDATE ingestion_identity_decisions SET state='rejected',error_code='attempts_exhausted',lease_until=NULL WHERE id=$1`, d.ID)
				return e
			}
			var claim pgx.Batch
			if d.ID == "" {
				grant, method := authority.grant, authority.method
				d = Decision{ID: uuid.NewString(), ReceiptID: receipt.ID, Generation: receipt.Generation, Method: method, GrantID: &grant.ID, GrantGeneration: &grant.Generation}
				claim.Queue(`INSERT INTO ingestion_identity_decisions(id,receipt_id,generation,method,grant_id,grant_generation)
					VALUES($1,$2,$3,$4,$5,$6)`, d.ID, receipt.ID, receipt.Generation, k.JSON(method), grant.ID, grant.Generation)
			}
			execution := uuid.NewString()
			var machineID string
			claim.Queue(`WITH lease AS (
				UPDATE ingestion_identity_decisions SET state='identifying',fence=fence+1,attempts=attempts+1,
				lease_until=clock_timestamp()+interval '45 seconds' WHERE id=$1 RETURNING id,fence,attempts)
				INSERT INTO ingestion_attempts(execution_id,stage,target_id,fence,attempt)
				SELECT $2,'identify',id,fence,attempts FROM lease RETURNING fence,attempt,`+machineExecutionID, d.ID, execution).
				QueryRow(func(row pgx.Row) error { return row.Scan(&d.Fence, &d.Attempts, &machineID) })
			var obj object
			claim.Queue(availableObject, receipt.ObjectID).QueryRow(func(row pgx.Row) error {
				return row.Scan(&obj.ID, &obj.Key, &obj.Hash, &obj.Size)
			})
			claim.Queue(`UPDATE ingestion_receipts SET state='identifying',error_code='' WHERE id=$1`, receipt.ID)
			if e = tx.SendBatch(ctx, &claim).Close(); e != nil {
				return e
			}
			picked = &identifyClaim{Source: authority.revision.Snapshot, Receipt: receipt, Decision: d, Input: obj, ExecutionID: execution, MachineID: machineID}
			return nil
		})
		if e != nil {
			return nil, e
		}
		if picked != nil {
			return picked, nil
		}
	}
	return nil, nil
}
func (s *Service) IdentifyOne(ctx context.Context) (bool, error) { return s.identify(ctx, nil) }
func (s *Service) identify(ctx context.Context, key *workKey) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	job, e := s.claimIdentity(ctx, key)
	if e != nil || job == nil {
		return false, e
	}
	raw, callErr := s.readObject(ctx, job.Input)
	var response i.Response
	var responseBytes []byte
	if callErr == nil {
		callErr = s.machineTx(ctx, *job.Receipt.TenantID, false, func(tx pgx.Tx) error { return s.identityAllowed(ctx, tx, job.Receipt, job.Decision) })
	}
	if callErr == nil {
		req := i.Request{Version: 1, ExecutionID: job.MachineID, Method: job.Decision.Method.Method, Input: i.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), ByteLength: int64(len(raw)), SHA256: job.Input.Hash}, Context: i.Context{FixedTerminal: job.Source.FixedTerminal, ExclusiveAuthenticated: job.Source.Exclusive, ReceivedAt: job.Receipt.ReceivedAt, Transport: job.Decision.Method.Transport, InputUnit: job.Decision.Method.InputUnit}, TraceID: job.Receipt.ID}
		started := time.Now()
		responseBytes, response, callErr = s.parser.Identify(parsing.MachineWork(ctx, "tenant:"+*job.Receipt.TenantID, job.ExecutionID, true), req, job.Decision.Method)
		s.parserDuration(started)
		observeDependency(ctx, "parser", callErr)
	}
	var stored *object
	if len(responseBytes) > 0 {
		obj, err := s.stageResponse(ctx, responseBytes)
		if err != nil {
			callErr = err
		} else {
			stored = &obj
		}
	}
	e = s.machineTx(ctx, *job.Receipt.TenantID, false, func(tx pgx.Tx) error {
		state := "succeeded"
		if callErr != nil {
			state = "failed"
		}
		var objectID any
		if stored != nil {
			objectID = stored.ID
		}
		if e := finishAttemptTx(ctx, tx, job.ExecutionID, state, code(callErr), objectID); e != nil {
			return e
		}
		var live bool
		if e := tx.QueryRow(ctx, `SELECT state='identifying' AND fence=$2 AND lease_until>clock_timestamp() FROM ingestion_identity_decisions WHERE id=$1 FOR UPDATE`, job.Decision.ID, job.Decision.Fence).Scan(&live); e != nil || !live {
			return e
		}
		if authErr := s.identityAllowed(ctx, tx, job.Receipt, job.Decision); authErr != nil {
			callErr = authErr
		}
		status, errorCode := response.Status, code(callErr)
		if callErr != nil {
			status = "rejected"
			if retryableAttempt(callErr) && job.Decision.Attempts < 5 {
				status = "retry_wait"
			} else if errorCode == "authorization_revoked" || errorCode == "execution_paused" {
				status = "blocked"
			}
		}
		if callErr == nil && response.Status != "identified" {
			errorCode = "identity_rejected"
			if len(response.Diagnostics) > 0 {
				errorCode = response.Diagnostics[0]
			}
		}
		var effects pgx.Batch
		effects.Queue(`UPDATE ingestion_identity_decisions SET state=$2,error_code=$3,response_object_id=$4,response=$5,
			lease_until=NULL,available_at=clock_timestamp()+$6*interval '1 second' WHERE id=$1`,
			job.Decision.ID, status, errorCode, objectID, k.JSON(response), retryDelay(job.Decision.Attempts))
		receiptState := status
		if status == "identified" {
			for _, unit := range response.Units {
				effects.Queue(`INSERT INTO ingestion_units(receipt_id,decision_id,unit_key,terminal_key,child_key,view,message_kind)
					VALUES($1,$2,$3,$4,$5,$6,$7)`, job.Receipt.ID, job.Decision.ID, unit.Key, unit.TerminalKey, unit.ChildKey, k.JSON(unit.View), unit.MessageKind)
			}
			receiptState = "identified"
		} else if status == "rejected" {
			receiptState = "awaiting_identity"
		}
		effects.Queue(`UPDATE ingestion_receipts SET state=$2,error_code=$3 WHERE id=$1`, job.Receipt.ID, receiptState, errorCode)
		return key.terminalWrite(tx.SendBatch(ctx, &effects).Close(), receiptState == "identified" || receiptState == "awaiting_identity")
	})
	return true, e
}
func retryDelay(attempt int) int {
	switch attempt {
	case 1:
		return 1
	case 2:
		return 5
	case 3:
		return 30
	default:
		return 120
	}
}

func identitySlotTx(ctx context.Context, tx pgx.Tx, tenant string) (bool, error) {
	// The count takes a new READ COMMITTED snapshot after the previous owner
	// releases its lock. Keep these two ordered statements in one round trip.
	var batch pgx.Batch
	batch.Queue(`SELECT pg_advisory_xact_lock(754043)`)
	var all, own int
	batch.Queue(`SELECT count(*),count(*) FILTER(WHERE tenant_id=$1::uuid) FROM (
		SELECT r.tenant_id FROM ingestion_identity_decisions d JOIN ingestion_receipts r ON r.id=d.receipt_id WHERE d.state='identifying' AND d.lease_until>clock_timestamp()
		UNION ALL SELECT r.tenant_id FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE u.state='demultiplexing' AND u.lease_until>clock_timestamp()
		) running`, tenant).QueryRow(func(row pgx.Row) error { return row.Scan(&all, &own) })
	e := tx.SendBatch(ctx, &batch).Close()
	return all < 4 && own < 2, e
}
