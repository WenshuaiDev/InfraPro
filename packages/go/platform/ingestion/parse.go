package ingestion

import (
	"context"
	"encoding/base64"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type parseClaim struct {
	Plan        Plan
	Input       object
	ExecutionID string
}

func (s *Service) planAllowed(ctx context.Context, tx pgx.Tx, p Plan) error {
	if p.Snapshot.RecoveryID != "" {
		if e := s.recoveryAllowedTx(ctx, tx, p.Snapshot.RecoveryID, p.Snapshot.RecoveryGeneration); e != nil {
			return e
		}
	}
	if !s.options.Enabled {
		return c.Fail("ingestion_disabled")
	}
	src, e := sourceTx(ctx, tx, k.Tenant(p.TenantID), p.Snapshot.Source.IngressID)
	if e != nil {
		return e
	}
	if src.Blocked || src.PlanningPaused && p.Snapshot.RecoveryID == "" {
		return c.Fail("execution_paused")
	}
	if _, e = businessTx(ctx, tx, src, p.DeviceID); e != nil {
		return e
	}
	for _, saved := range []Grant{p.Snapshot.SourceGrant, p.Snapshot.Grant} {
		var terminal string
		if saved.TerminalID != nil {
			terminal = *saved.TerminalID
		}
		current, e := grantTx(ctx, tx, saved.SourceID, terminal)
		if e != nil {
			return e
		}
		if !current.Enabled || current.ID != saved.ID || current.Generation != saved.Generation || terminal == "" && !current.Identify || terminal != "" && !current.Measure {
			return c.Fail("authorization_revoked")
		}
	}
	protocol, e := s.parser.IngressProtocolTx(ctx, tx, p.Snapshot.Source.ProtocolID, p.Snapshot.Source.ProtocolVersion, false)
	if e != nil {
		return e
	}
	compat, e := s.parser.CompatibilityTx(ctx, tx, protocol, p.Snapshot.Association.Demultiplexer, p.Snapshot.Input.InputUnit, p.Snapshot.Parsing.Snapshot.Method, false)
	if e != nil {
		return e
	}
	if compat.Digest != p.Snapshot.Compatibility.Digest {
		return c.Fail("ingress_method_incompatible")
	}
	if p.Snapshot.Association.Demultiplexer != nil {
		if _, e = s.parser.IdentificationTx(ctx, tx, *p.Snapshot.Association.Demultiplexer, false); e != nil {
			return e
		}
	}
	return s.parser.ValidateProductionTx(ctx, tx, p.TenantID, p.Snapshot.Parsing)
}
func (s *Service) claimParse(ctx context.Context) (*parseClaim, error) {
	var candidates []Plan
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `WITH busy AS MATERIALIZED (
 SELECT tenant_id,count(*) n FROM ingestion_parse_plans WHERE state='parsing' AND lease_until>clock_timestamp() GROUP BY tenant_id
), eligible AS MATERIALIZED (
 SELECT src.id,src.planning_paused FROM ingestion_sources src LEFT JOIN busy ON busy.tenant_id=src.tenant_id WHERE NOT src.blocked AND COALESCE(busy.n,0)<2
)
SELECT candidate.id::text,candidate.tenant_id::text FROM eligible src CROSS JOIN LATERAL (
 SELECT p.id,p.tenant_id,p.available_at FROM ingestion_parse_plans p
 WHERE (p.snapshot->'source'->>'ingress_id')::uuid=src.id
 AND p.state IN ('queued','retry_wait','parsing')
 AND (NOT src.planning_paused OR COALESCE(p.snapshot->>'recovery_id','')<>'')
 AND ((p.state IN ('queued','retry_wait') AND p.available_at<=clock_timestamp()) OR (p.state='parsing' AND p.lease_until<clock_timestamp()))
 ORDER BY p.available_at LIMIT 16
) candidate ORDER BY candidate.available_at LIMIT 16`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var p Plan
			if e := rows.Scan(&p.ID, &p.TenantID); e != nil {
				return e
			}
			candidates = append(candidates, p)
		}
		return rows.Err()
	})
	if e != nil {
		return nil, e
	}
	for _, candidate := range candidates {
		var picked *parseClaim
		e = s.machineTx(ctx, candidate.TenantID, false, func(tx pgx.Tx) error {
			var live bool
			if e := tx.QueryRow(ctx, `SELECT state IN ('queued','retry_wait') AND available_at<=clock_timestamp() OR state='parsing' AND lease_until<clock_timestamp() FROM ingestion_parse_plans WHERE id=$1 FOR UPDATE`, candidate.ID).Scan(&live); e != nil || !live {
				return e
			}
			p, e := read[Plan](tx.QueryRow(ctx, `SELECT `+planJSON+` FROM ingestion_parse_plans p WHERE id=$1`, candidate.ID))
			if e != nil {
				return e
			}
			if e = s.planAllowed(ctx, tx, p); e != nil {
				_, err := tx.Exec(ctx, `UPDATE ingestion_parse_plans SET state='blocked',error_code=$2,lease_until=NULL,finished_at=clock_timestamp() WHERE id=$1`, p.ID, code(e))
				return err
			}
			if p.Attempts >= 5 {
				_, e = tx.Exec(ctx, `UPDATE ingestion_parse_plans SET state='rejected',error_code='attempts_exhausted',lease_until=NULL,finished_at=clock_timestamp() WHERE id=$1`, p.ID)
				return e
			}
			if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(754044)`); e != nil {
				return e
			}
			var global, tenant int
			if e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE tenant_id=$1) FROM ingestion_parse_plans WHERE state='parsing' AND lease_until>clock_timestamp()`, p.TenantID).Scan(&global, &tenant); e != nil {
				return e
			}
			if global >= 4 || tenant >= 2 {
				return nil
			}
			if e = tx.QueryRow(ctx, `UPDATE ingestion_parse_plans SET state='parsing',fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+interval '45 seconds' WHERE id=$1 RETURNING fence,attempts`, p.ID).Scan(&p.Fence, &p.Attempts); e != nil {
				return e
			}
			receipt, e := receiptTx(ctx, tx, p.Snapshot.ReceiptID)
			if e != nil {
				return e
			}
			obj, e := objectTx(ctx, tx, receipt.ObjectID)
			if e != nil {
				return e
			}
			execution := uuid.NewString()
			if _, e = tx.Exec(ctx, `INSERT INTO ingestion_attempts(execution_id,stage,target_id,fence,attempt) VALUES($1,'parse',$2,$3,$4)`, execution, p.ID, p.Fence, p.Attempts); e != nil {
				return e
			}
			picked = &parseClaim{Plan: p, Input: obj, ExecutionID: execution}
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
func (s *Service) ParseOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	job, e := s.claimParse(ctx)
	if e != nil || job == nil {
		return false, e
	}
	p := job.Plan
	raw, callErr := s.readObject(ctx, job.Input)
	if callErr == nil {
		raw, callErr = i.Materialize(raw, p.Snapshot.Input)
	}
	if callErr == nil {
		callErr = s.machineTx(ctx, p.TenantID, false, func(tx pgx.Tx) error { return s.planAllowed(ctx, tx, p) })
	}
	var body []byte
	var response c.Response
	if callErr == nil {
		req := c.Request{ContractVersion: 1, ExecutionID: job.ExecutionID, Method: p.Snapshot.Parsing.Entry.Method(), Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), SHA256: c.Digest(raw), ByteLength: len(raw)}, Context: c.Context{ReceivedAt: p.Snapshot.ReceivedAt, Transport: p.Snapshot.Input.ParseTransport}, TraceID: p.Snapshot.TraceID}
		body, response, callErr = s.parser.ExecuteProduction(ctx, req, p.Snapshot.Parsing.Entry)
		observeDependency(ctx, "parser", callErr)
		if callErr == nil {
			for _, id := range response.Identities {
				if id.Role == "dtu" && id.Text != p.Snapshot.Terminal.Key {
					callErr = c.Fail("parsed_identity_mismatch")
				}
				if id.Role == "child" && id.Text != p.Snapshot.Association.ChildKey {
					callErr = c.Fail("parsed_identity_mismatch")
				}
			}
		}
	}
	var stored *object
	if len(body) > 0 {
		obj, err := s.saveObject(ctx, body)
		if err != nil {
			callErr = err
		} else {
			stored = &obj
		}
	}
	e = s.machineTx(ctx, p.TenantID, false, func(tx pgx.Tx) error {
		var objectID any
		if stored != nil {
			objectID = stored.ID
		}
		attemptState := "succeeded"
		if callErr != nil {
			attemptState = "failed"
		}
		if _, e := tx.Exec(ctx, `UPDATE ingestion_attempts SET state=$2,error_code=$3,response_object_id=$4,finished_at=clock_timestamp() WHERE execution_id=$1`, job.ExecutionID, attemptState, code(callErr), objectID); e != nil {
			return e
		}
		var live bool
		if e := tx.QueryRow(ctx, `SELECT state='parsing' AND fence=$2 AND lease_until>clock_timestamp() FROM ingestion_parse_plans WHERE id=$1 FOR UPDATE`, p.ID, p.Fence).Scan(&live); e != nil || !live {
			return e
		}
		if authErr := s.planAllowed(ctx, tx, p); authErr != nil {
			callErr = authErr
		}
		state, errorCode := response.Status, code(callErr)
		if callErr != nil {
			state = "rejected"
			if parsing.Retryable(callErr) && p.Attempts < 5 {
				state = "retry_wait"
			} else if errorCode == "authorization_revoked" || errorCode == "routing_scope_mismatch" || errorCode == "execution_paused" || errorCode == "method_blocked" {
				state = "blocked"
			}
		}
		if callErr == nil && response.Status != "parsed" {
			errorCode = response.Status
		}
		if callErr == nil && stored != nil {
			if _, e := tx.Exec(ctx, `INSERT INTO ingestion_results(plan_id,execution_id,response_object_id,status,mapping) VALUES($1,$2,$3,$4,$5) ON CONFLICT(plan_id) DO NOTHING`, p.ID, job.ExecutionID, stored.ID, response.Status, k.JSON(p.Snapshot.Parsing.Mapping)); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `UPDATE ingestion_parse_plans SET state=$2,error_code=$3,lease_until=NULL,available_at=clock_timestamp()+$4*interval '1 second',finished_at=CASE WHEN $2='retry_wait' THEN NULL ELSE clock_timestamp() END WHERE id=$1`, p.ID, state, errorCode, retryDelay(p.Attempts)); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE ingestion_units SET state=$2,error_code=$3 WHERE id=$1`, p.UnitID, state, errorCode)
		return e
	})
	return true, e
}
