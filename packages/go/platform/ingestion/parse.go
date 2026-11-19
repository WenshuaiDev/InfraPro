package ingestion

import (
	"context"
	_ "embed"
	"encoding/base64"
	"strings"
	"time"

	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

//go:embed candidates/parse.sql
var parseCandidates string

type parseClaim struct {
	Plan        Plan
	Input       object
	ExecutionID string
	MachineID   string
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
	// The tenant and catalog locks remain held across this choice and the
	// operation. Resolve the active route and both fixed grant revisions in a
	// single statement, preserving the original failure order.
	var blocked, paused, active bool
	var deviceGrant *string
	var grantChecks []*bool
	sourceIDs := []string{p.Snapshot.SourceGrant.SourceID, p.Snapshot.Grant.SourceID}
	terminals := []string{"", ""}
	for n, g := range []Grant{p.Snapshot.SourceGrant, p.Snapshot.Grant} {
		if g.TerminalID != nil {
			terminals[n] = *g.TerminalID
		}
	}
	e := tx.QueryRow(ctx, `SELECT s.blocked,s.planning_paused,`+activeRoutingProject+`,`+routingDeviceGrant+`,
		ARRAY(SELECT g.enabled AND g.id::text=ref.id AND g.generation::text=ref.generation
			AND CASE WHEN ref.terminal='' THEN g.identify ELSE g.measure END
			FROM unnest($4::text[],$5::text[],$6::text[],$7::text[]) WITH ORDINALITY AS ref(source,terminal,id,generation,ord)
			LEFT JOIN ingestion_execution_grants g ON g.source_id=ref.source::uuid AND g.terminal_id IS NOT DISTINCT FROM NULLIF(ref.terminal,'')::uuid
			ORDER BY ref.ord)
		FROM ingestion_sources s WHERE s.id=$1 AND s.tenant_id=$3`, p.Snapshot.Source.IngressID, p.DeviceID, p.TenantID,
		sourceIDs, terminals, []string{p.Snapshot.SourceGrant.ID, p.Snapshot.Grant.ID}, []string{p.Snapshot.SourceGrant.Generation, p.Snapshot.Grant.Generation}).
		Scan(&blocked, &paused, &active, &deviceGrant, &grantChecks)
	if e == pgx.ErrNoRows {
		return k.NotFound("ingestion")
	}
	if e != nil {
		return e
	}
	if blocked || paused && p.Snapshot.RecoveryID == "" {
		return c.Fail("execution_paused")
	}
	if _, e = checkedRoute(active, p.DeviceID, deviceGrant); e != nil {
		return e
	}
	for _, allowed := range grantChecks {
		if allowed == nil {
			return k.NotFound("ingestion")
		}
		if !*allowed {
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
func (s *Service) claimParse(ctx context.Context, key *workKey) (*parseClaim, error) {
	var candidates []Plan
	var e error
	if key != nil {
		candidates = []Plan{{ID: key.id, TenantID: key.scope}}
	} else {
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			unrecorded := s.unrecorded("parsing-dispatch-worker", "ingestion.plan.ready", "parse", "p.id", "(SELECT generation FROM ingestion_identity_decisions WHERE id=p.decision_id)")
			query := strings.ReplaceAll(parseCandidates, "/* split event ownership */", unrecorded)
			s.recordCandidateScan()
			rows, e := tx.Query(ctx, query)
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
	}
	for _, candidate := range candidates {
		var picked *parseClaim
		e = s.machineTx(ctx, candidate.TenantID, false, func(tx pgx.Tx) error {
			var p Plan
			var live bool
			var e error
			if key != nil {
				e = tx.QueryRow(ctx, `SELECT `+planJSON+` FROM ingestion_parse_plans p
					JOIN ingestion_sources src ON src.id=(p.snapshot->'source'->>'ingress_id')::uuid
					JOIN ingestion_identity_decisions d ON d.id=p.decision_id WHERE p.id=$1 AND p.tenant_id=$2 AND d.generation=$3
					AND NOT src.blocked AND (NOT src.planning_paused OR COALESCE(p.snapshot->>'recovery_id','')<>'')
					AND (p.state IN ('queued','retry_wait') AND p.available_at<=clock_timestamp() OR p.state='parsing' AND p.lease_until<clock_timestamp())
					FOR UPDATE OF p`, key.id, key.scope, key.generation).Scan(&p)
				if e == pgx.ErrNoRows {
					return nil
				}
				live = true
			} else {
				e = tx.QueryRow(ctx, `SELECT `+planJSON+`,state IN ('queued','retry_wait') AND available_at<=clock_timestamp() OR state='parsing' AND lease_until<clock_timestamp()
					FROM ingestion_parse_plans p WHERE id=$1 FOR UPDATE`, candidate.ID).Scan(&p, &live)
			}
			if e != nil || !live {
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
			var obj object
			e = tx.QueryRow(ctx, `SELECT o.id::text,o.object_key,o.sha256,o.byte_length
				FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id AND o.state='available'
				WHERE r.id=$1`, p.Snapshot.ReceiptID).Scan(&obj.ID, &obj.Key, &obj.Hash, &obj.Size)
			if e == pgx.ErrNoRows {
				// Distinguish a stale receipt reference from a missing available
				// object exactly as the original two lookups did.
				_, receiptErr := receiptTx(ctx, tx, p.Snapshot.ReceiptID)
				if receiptErr != nil {
					return receiptErr
				}
			}
			if e != nil {
				return e
			}
			execution := uuid.NewString()
			machineID, e := recordAttempt(ctx, tx, "parse", p.ID, execution, p.Fence, p.Attempts)
			if e != nil {
				return e
			}
			picked = &parseClaim{Plan: p, Input: obj, ExecutionID: execution, MachineID: machineID}
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
func (s *Service) ParseOne(ctx context.Context) (bool, error) { return s.parse(ctx, nil) }
func (s *Service) parse(ctx context.Context, key *workKey) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	job, e := s.claimParse(ctx, key)
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
		req := c.Request{ContractVersion: 1, ExecutionID: job.MachineID, Method: p.Snapshot.Parsing.Entry.Method(), Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), SHA256: c.Digest(raw), ByteLength: len(raw)}, Context: c.Context{ReceivedAt: p.Snapshot.ReceivedAt, Transport: p.Snapshot.Input.ParseTransport}, TraceID: p.Snapshot.TraceID}
		started := time.Now()
		body, response, callErr = s.parser.ExecuteProduction(parsing.MachineWork(ctx, "tenant:"+p.TenantID, job.ExecutionID, true), req, p.Snapshot.Parsing.Entry)
		s.parserDuration(started)
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
		obj, err := s.stageResponse(ctx, body)
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
		if e := finishAttemptTx(ctx, tx, job.ExecutionID, attemptState, code(callErr), objectID); e != nil {
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
			if retryableAttempt(callErr) && p.Attempts < 5 {
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
		return key.terminalWrite(e, state != "retry_wait")
	})
	return true, e
}
