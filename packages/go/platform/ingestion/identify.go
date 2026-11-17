package ingestion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type identifyClaim struct {
	Receipt     Receipt
	Decision    Decision
	Input       object
	ExecutionID string
	Source      r.Source
}

func (s *Service) identityAllowed(ctx context.Context, tx pgx.Tx, receipt Receipt, d Decision) error {
	if d.RecoveryID != nil {
		if d.RecoveryGeneration == nil {
			return c.Fail("recovery_not_active")
		}
		if e := s.recoveryAllowedTx(ctx, tx, *d.RecoveryID, *d.RecoveryGeneration); e != nil {
			return e
		}
	}
	if receipt.TenantID == nil {
		return c.Fail("source_revision_unknown")
	}
	src, e := sourceTx(ctx, tx, k.Tenant(*receipt.TenantID), receipt.SourceID)
	if e != nil {
		return e
	}
	if src.Blocked || src.PlanningPaused && d.RecoveryID == nil {
		return c.Fail("execution_paused")
	}
	if _, e = businessTx(ctx, tx, src, ""); e != nil {
		return e
	}
	var changing bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingestion_change_intents WHERE source_id=$1 AND state='pending')`, src.ID).Scan(&changing); e != nil {
		return e
	}
	if changing {
		return c.Fail("association_changing")
	}
	rev, e := revisionTx(ctx, tx, src.ID, receipt.SourceRevision)
	if e != nil {
		return e
	}
	if !rev.Confirmed {
		return c.Fail("awaiting_source_confirmation")
	}
	if _, e = s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false); e != nil {
		return e
	}
	g, e := grantTx(ctx, tx, src.ID, "")
	if e != nil {
		return c.Fail("execution_unconfigured")
	}
	if !g.Enabled || !g.Identify || d.GrantID != nil && (*d.GrantID != g.ID || d.GrantGeneration == nil || *d.GrantGeneration != g.Generation) {
		return c.Fail("authorization_revoked")
	}
	return nil
}
func (s *Service) claimIdentity(ctx context.Context) (*identifyClaim, error) {
	var candidates []Receipt
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `WITH busy AS MATERIALIZED (
 SELECT br.tenant_id,count(*) n FROM ingestion_identity_decisions d JOIN ingestion_receipts br ON br.id=d.receipt_id WHERE d.state='identifying' AND d.lease_until>clock_timestamp() GROUP BY br.tenant_id
), eligible AS MATERIALIZED (
 SELECT s.id,s.tenant_id,s.planning_paused FROM ingestion_sources s LEFT JOIN busy ON busy.tenant_id=s.tenant_id
 WHERE COALESCE(busy.n,0)<2 AND NOT s.blocked AND NOT EXISTS(SELECT 1 FROM ingestion_change_intents c WHERE c.source_id=s.id AND c.state='pending')
), fresh AS MATERIALIZED (
 SELECT picked.* FROM eligible s CROSS JOIN LATERAL (
  SELECT r.id,r.tenant_id,r.stored_at available_at FROM ingestion_receipts r
  WHERE NOT s.planning_paused AND r.source_id=s.id AND r.tenant_id IS NOT NULL AND r.state='stored'
  AND NOT EXISTS(SELECT 1 FROM ingestion_identity_decisions d WHERE d.receipt_id=r.id AND d.generation=r.current_generation)
  ORDER BY r.stored_at LIMIT 16
 ) picked
), retries AS MATERIALIZED (
 SELECT r.id,r.tenant_id,d.available_at FROM ingestion_identity_decisions d JOIN ingestion_receipts r ON r.id=d.receipt_id AND r.current_generation=d.generation JOIN eligible s ON s.id=r.source_id
 WHERE r.tenant_id IS NOT NULL AND r.state IN ('stored','identifying','retry_wait') AND (NOT s.planning_paused OR d.recovery_id IS NOT NULL)
 AND (d.state IN ('queued','retry_wait') AND d.available_at<=clock_timestamp() OR d.state='identifying' AND d.lease_until<clock_timestamp()) ORDER BY d.available_at LIMIT 16
)
SELECT id::text,tenant_id::text FROM (SELECT * FROM fresh UNION ALL SELECT * FROM retries) candidates ORDER BY available_at LIMIT 16`)
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
	for _, candidate := range candidates {
		var picked *identifyClaim
		e = s.machineTx(ctx, *candidate.TenantID, false, func(tx pgx.Tx) error {
			// Lock first, then take a fresh statement snapshot for the decision.
			// A correlated EXISTS evaluated in SELECT ... FOR UPDATE can retain
			// a pre-wait decision snapshot after another claimant changes its lease.
			if _, e := tx.Exec(ctx, `SELECT id FROM ingestion_receipts WHERE id=$1 FOR UPDATE`, candidate.ID); e != nil {
				return e
			}
			receipt, e := receiptTx(ctx, tx, candidate.ID)
			if e != nil {
				return e
			}
			var live bool
			if e = tx.QueryRow(ctx, `SELECT state='stored' OR EXISTS(SELECT 1 FROM ingestion_identity_decisions d WHERE d.receipt_id=r.id AND d.generation=r.current_generation AND (d.state IN ('queued','retry_wait') AND d.available_at<=clock_timestamp() OR d.state='identifying' AND d.lease_until<clock_timestamp())) FROM ingestion_receipts r WHERE id=$1`, receipt.ID).Scan(&live); e != nil || !live {
				return e
			}
			d := Decision{}
			var raw []byte
			e = tx.QueryRow(ctx, `SELECT `+decisionJSON+` FROM ingestion_identity_decisions d WHERE receipt_id=$1 AND generation=$2`, receipt.ID, receipt.Generation).Scan(&raw)
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			if e == nil {
				if e = json.Unmarshal(raw, &d); e != nil {
					return e
				}
			}
			if e = s.identityAllowed(ctx, tx, receipt, d); e != nil {
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

			if d.ID == "" {
				rev, e := revisionTx(ctx, tx, receipt.SourceID, receipt.SourceRevision)
				if e != nil {
					return e
				}
				protocol, e := s.parser.IngressProtocolTx(ctx, tx, rev.Snapshot.ProtocolID, rev.Snapshot.ProtocolVersion, false)
				if e != nil {
					return e
				}
				method, e := s.parser.IdentificationTx(ctx, tx, protocol.Identification, false)
				if e != nil {
					return e
				}
				grant, e := grantTx(ctx, tx, receipt.SourceID, "")
				if e != nil {
					return e
				}
				d = Decision{ID: uuid.NewString(), ReceiptID: receipt.ID, Generation: receipt.Generation, Method: method, GrantID: &grant.ID, GrantGeneration: &grant.Generation}
				if _, e = tx.Exec(ctx, `INSERT INTO ingestion_identity_decisions(id,receipt_id,generation,method,grant_id,grant_generation) VALUES($1,$2,$3,$4,$5,$6)`, d.ID, receipt.ID, receipt.Generation, k.JSON(method), grant.ID, grant.Generation); e != nil {
					return e
				}
			}
			if d.Attempts >= 5 {
				_, e = tx.Exec(ctx, `UPDATE ingestion_identity_decisions SET state='rejected',error_code='attempts_exhausted',lease_until=NULL WHERE id=$1`, d.ID)
				return e
			}
			if e = tx.QueryRow(ctx, `UPDATE ingestion_identity_decisions SET state='identifying',fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+interval '45 seconds' WHERE id=$1 RETURNING fence,attempts`, d.ID).Scan(&d.Fence, &d.Attempts); e != nil {
				return e
			}
			execution := uuid.NewString()
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_attempts(execution_id,stage,target_id,fence,attempt) VALUES($1,'identify',$2,$3,$4)`, execution, d.ID, d.Fence, d.Attempts); e != nil {
				return e
			}
			obj, e := objectTx(ctx, tx, receipt.ObjectID)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE ingestion_receipts SET state='identifying',error_code='' WHERE id=$1`, receipt.ID); e != nil {
				return e
			}
			sourceRev, e := revisionTx(ctx, tx, receipt.SourceID, receipt.SourceRevision)
			if e != nil {
				return e
			}
			picked = &identifyClaim{Source: sourceRev.Snapshot, Receipt: receipt, Decision: d, Input: obj, ExecutionID: execution}
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
func (s *Service) IdentifyOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	job, e := s.claimIdentity(ctx)
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
		req := i.Request{Version: 1, ExecutionID: job.ExecutionID, Method: job.Decision.Method.Method, Input: i.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), ByteLength: int64(len(raw)), SHA256: job.Input.Hash}, Context: i.Context{FixedTerminal: job.Source.FixedTerminal, ExclusiveAuthenticated: job.Source.Exclusive, ReceivedAt: job.Receipt.ReceivedAt, Transport: job.Decision.Method.Transport, InputUnit: job.Decision.Method.InputUnit}, TraceID: job.Receipt.ID}
		responseBytes, response, callErr = s.parser.Identify(ctx, req, job.Decision.Method)
		observeDependency(ctx, "parser", callErr)
	}
	var stored *object
	if len(responseBytes) > 0 {
		obj, err := s.saveObject(ctx, responseBytes)
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
		if _, e := tx.Exec(ctx, `UPDATE ingestion_attempts SET state=$2,error_code=$3,response_object_id=$4,finished_at=clock_timestamp() WHERE execution_id=$1`, job.ExecutionID, state, code(callErr), objectID); e != nil {
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
			if parsing.Retryable(callErr) && job.Decision.Attempts < 5 {
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
		if _, e := tx.Exec(ctx, `UPDATE ingestion_identity_decisions SET state=$2,error_code=$3,response_object_id=$4,response=$5,lease_until=NULL,available_at=clock_timestamp()+$6*interval '1 second' WHERE id=$1`, job.Decision.ID, status, errorCode, objectID, k.JSON(response), retryDelay(job.Decision.Attempts)); e != nil {
			return e
		}
		receiptState := status
		if status == "identified" {
			for _, unit := range response.Units {
				if _, e := tx.Exec(ctx, `INSERT INTO ingestion_units(receipt_id,decision_id,unit_key,terminal_key,child_key,view,message_kind) VALUES($1,$2,$3,$4,$5,$6,$7)`, job.Receipt.ID, job.Decision.ID, unit.Key, unit.TerminalKey, unit.ChildKey, k.JSON(unit.View), unit.MessageKind); e != nil {
					return e
				}
			}
			receiptState = "identified"
		} else if status == "rejected" {
			receiptState = "awaiting_identity"
		}
		_, e := tx.Exec(ctx, `UPDATE ingestion_receipts SET state=$2,error_code=$3 WHERE id=$1`, job.Receipt.ID, receiptState, errorCode)
		return e
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
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(754043)`); e != nil {
		return false, e
	}
	var all, own int
	e := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE tenant_id=$1::uuid) FROM (SELECT r.tenant_id FROM ingestion_identity_decisions d JOIN ingestion_receipts r ON r.id=d.receipt_id WHERE d.state='identifying' AND d.lease_until>clock_timestamp() UNION ALL SELECT r.tenant_id FROM ingestion_units u JOIN ingestion_receipts r ON r.id=u.receipt_id WHERE u.state='demultiplexing' AND u.lease_until>clock_timestamp()) running`, tenant).Scan(&all, &own)
	return all < 4 && own < 2, e
}
