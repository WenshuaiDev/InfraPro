package parsing

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"sync"
	"time"
)

type claim struct {
	Run       Run
	Execution Execution
}

// workerTx can record authorization_revoked without ever borrowing a worker's
// business permission. It follows the same graph/catalog/tenant lock order.
func (s *Service) workerTx(ctx context.Context, r Run, actor string, fn func(pgx.Tx, k.Authorization, error) error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, actor); e != nil {
			return e
		}
		a := workerActor(actor)
		authorizationErr := a.Check(ctx, tx, false)
		auth := k.Authorization{Scope: runScope(r), Account: actor}
		if authorizationErr == nil {
			auth, authorizationErr = k.AuthorizeTx(ctx, tx, a, runScope(r), op(runScope(r), "runs", "create"))
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, runScope(r).Entry == "platform"); e != nil {
			return e
		}
		if r.TenantID != nil {
			if e := devicecoord.Tenant(ctx, tx, *r.TenantID, true); e != nil {
				return e
			}
		}
		if authorizationErr == nil && r.Kind != "" {
			authorizationErr = s.authorizeRunTx(ctx, tx, auth, r, true)
		}
		return fn(tx, auth, authorizationErr)
	})
}
func (s *Service) claim(ctx context.Context) (*claim, error) {
	var candidates []claim
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT r.id::text,r.tenant_id::text,e.actor_id::text FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE e.state='queued' OR (e.state='running' AND e.lease_until<clock_timestamp()) ORDER BY e.created_at LIMIT 100`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c claim
			if e = rows.Scan(&c.Run.ID, &c.Run.TenantID, &c.Execution.ActorID); e != nil {
				return e
			}
			candidates = append(candidates, c)
		}
		return rows.Err()
	})
	if e != nil {
		return nil, e
	}
	for _, candidate := range candidates {
		var selected *claim
		e = s.workerTx(ctx, candidate.Run, candidate.Execution.ActorID, func(tx pgx.Tx, auth k.Authorization, authorizationErr error) error {
			r, e := runTx(ctx, tx, runScope(candidate.Run), candidate.Run.ID)
			if e != nil {
				return e
			}
			x, e := executionTx(ctx, tx, r.ID, r.Generation)
			if e != nil {
				return e
			}
			if x.ActorID != candidate.Execution.ActorID {
				return nil
			}
			if x.State != "queued" && x.State != "running" {
				return nil
			}
			var claimable bool
			if e = tx.QueryRow(ctx, `SELECT state='queued' OR lease_until<clock_timestamp() FROM parsing_run_executions WHERE run_id=$1 AND generation=$2 FOR UPDATE`, r.ID, r.Generation).Scan(&claimable); e != nil {
				return e
			}
			if !claimable {
				return nil
			}
			// The authorization above used only candidate identity; check full input now.
			if authorizationErr == nil {
				authorizationErr = s.authorizeRunTx(ctx, tx, auth, r, true)
			}
			failure := ""
			if authorizationErr != nil {
				failure = "authorization_revoked"
			}
			m, e := methodTx(ctx, tx, r.Input.Entry.MethodID, r.Input.Entry.MethodVersion)
			if e != nil {
				return e
			}
			if m.State == "blocked" {
				failure = "method_blocked"
			}
			if len(x.Attempts) >= 3 {
				failure = "attempts_exhausted"
			}
			if failure != "" {
				return s.terminalTx(ctx, tx, r, x, "failed", failure, nil, "", nil)
			}
			var running int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1 AND e.state='running' AND e.lease_until>clock_timestamp()`, scopeKey(auth.Scope)).Scan(&running); e != nil {
				return e
			}
			if running >= s.options.Concurrency {
				return nil
			}
			if _, e = tx.Exec(ctx, `UPDATE parsing_run_executions SET state='running',fence=fence+1,lease_until=clock_timestamp()+interval '45 seconds',started_at=COALESCE(started_at,clock_timestamp()) WHERE run_id=$1 AND generation=$2`, r.ID, r.Generation); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE parsing_runs SET revision=revision+1 WHERE id=$1`, r.ID); e != nil {
				return e
			}
			if _, e = publishTaskTx(ctx, tx, r.ID, "claimed", ""); e != nil {
				return e
			}
			x, e = executionTx(ctx, tx, r.ID, r.Generation)
			if e != nil {
				return e
			}
			selected = &claim{Run: r, Execution: x}
			return nil
		})
		if e != nil {
			return nil, e
		}
		if selected != nil {
			return selected, nil
		}
	}
	return nil, nil
}
func (s *Service) terminalTx(ctx context.Context, tx pgx.Tx, r Run, x Execution, state, code string, result *object, responseStatus string, mapping []Mapping) error {
	var objectID any
	if result != nil {
		objectID = result.ID
	}
	if mapping == nil {
		mapping = []Mapping{}
	}
	tag, e := tx.Exec(ctx, `UPDATE parsing_run_executions SET state=$4,error_code=$5,result_object_id=$6,response_status=NULLIF($7,''),mapping=$8,finished_at=clock_timestamp(),lease_until=NULL WHERE run_id=$1 AND generation=$2 AND fence=$3 AND state IN ('queued','running')`, r.ID, x.Generation, x.Fence, state, code, objectID, responseStatus, k.JSON(mapping))
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, e = tx.Exec(ctx, `UPDATE parsing_runs SET revision=revision+1,control_revision=control_revision+1 WHERE id=$1 AND current_generation=$2`, r.ID, x.Generation); e != nil {
		return e
	}
	_, e = publishTaskTx(ctx, tx, r.ID, state, "")
	return e
}
func (s *Service) RunOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled {
		return false, nil
	}
	job, e := s.claim(ctx)
	if e != nil || job == nil {
		return false, e
	}
	r, x := job.Run, job.Execution
	var inputObject object
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		sample, e := sampleTx(ctx, tx, runScope(r), r.SampleID)
		if e != nil {
			return e
		}
		inputObject, e = objectTx(ctx, tx, sample.ObjectID)
		return e
	})
	var raw []byte
	if e == nil {
		raw, e = s.readObject(ctx, inputObject, c.MaxInput)
	}
	if e == nil && (c.Digest(raw) != r.Input.Sample.SHA256 || len(raw) != r.Input.Sample.ByteLength) {
		e = k.Problem("input_hash_mismatch", "原样本摘要不符。", 409)
	}
	if e != nil {
		return true, s.finish(ctx, r, x, "failed", codeOf(e), nil, "", nil)
	}
	for attempt := len(x.Attempts) + 1; attempt <= 3; attempt++ {
		executionID := uuid.NewString()
		var resultObject object
		allowed := false
		e = s.workerTx(ctx, r, x.ActorID, func(tx pgx.Tx, _ k.Authorization, authorizationErr error) error {
			live, e := s.liveTx(ctx, tx, r, x)
			if e != nil || !live {
				return e
			}
			if authorizationErr != nil {
				return s.terminalTx(ctx, tx, r, x, "failed", "authorization_revoked", nil, "", nil)
			}
			m, e := methodTx(ctx, tx, r.Input.Entry.MethodID, r.Input.Entry.MethodVersion)
			if e != nil {
				return e
			}
			if m.State == "blocked" {
				return s.terminalTx(ctx, tx, r, x, "failed", "method_blocked", nil, "", nil)
			}
			resultObject, e = s.reserveTx(ctx, tx, runScope(r), c.MaxResponse)
			if e != nil {
				return s.terminalTx(ctx, tx, r, x, "failed", "storage_budget_exceeded", nil, "", nil)
			}
			if _, e = tx.Exec(ctx, `INSERT INTO parsing_run_attempts(execution_id,run_id,generation,attempt,fence,state) VALUES($1,$2,$3,$4,$5,'running')`, executionID, r.ID, x.Generation, attempt, x.Fence); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE parsing_run_executions SET lease_until=clock_timestamp()+interval '45 seconds' WHERE run_id=$1 AND generation=$2`, r.ID, x.Generation); e != nil {
				return e
			}
			allowed = true
			return nil
		})
		if e != nil || !allowed {
			return true, e
		}
		req := c.Request{ContractVersion: 1, ExecutionID: executionID, Method: r.Input.Entry.Method(), Input: c.Input{BytesBase64: base64.StdEncoding.EncodeToString(raw), SHA256: r.Input.Sample.SHA256, ByteLength: len(raw)}, Context: r.Input.Context, TraceID: r.Input.TraceID}
		callCtx, cancel := context.WithCancel(ctx)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			tick := time.NewTicker(300 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-callCtx.Done():
					return
				case <-tick.C:
					live := false
					err := s.access.Transact(callCtx, func(tx pgx.Tx) error { var e error; live, e = s.liveTx(callCtx, tx, r, x); return e })
					if err == nil && !live {
						cancel()
						return
					}
				}
			}
		}()
		responseBytes, response, callErr := s.ExecuteFixed(callCtx, req, r.Input.Entry)
		cancel()
		<-watchDone
		var stored *object
		if len(responseBytes) > 0 {
			resultObject.Hash = c.Digest(responseBytes)
			resultObject.Size = int64(len(responseBytes))
			if e = s.putImmutable(ctx, resultObject, responseBytes); e != nil {
				callErr = e
			} else {
				stored = &resultObject
			}
		}
		errorCode := codeOf(callErr)
		attemptState := "succeeded"
		if callErr != nil {
			attemptState = "failed"
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			var oid any
			if stored != nil {
				oid = stored.ID
				if _, e := tx.Exec(ctx, `UPDATE parsing_objects SET state='available',sha256=$2,size_bytes=$3 WHERE id=$1 AND state='intent'`, stored.ID, stored.Hash, stored.Size); e != nil {
					return e
				}
			} else {
				if _, e := tx.Exec(ctx, `UPDATE parsing_objects SET size_bytes=0 WHERE id=$1 AND state='intent'`, resultObject.ID); e != nil {
					return e
				}
			}
			_, e := tx.Exec(ctx, `UPDATE parsing_run_attempts SET state=$2,error_code=$3,response_object_id=$4,finished_at=clock_timestamp() WHERE execution_id=$1 AND state='running'`, executionID, attemptState, errorCode, oid)
			return e
		})
		if e != nil {
			return true, e
		}
		var remote *serviceError
		if errors.As(callErr, &remote) && remote.retry && attempt < 3 {
			delay := time.Second
			if attempt == 2 {
				delay = 3 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return true, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		state := "succeeded"
		if callErr != nil {
			state = "failed"
		} else if response.Status != "parsed" {
			state = "failed"
			errorCode = response.Status
		}
		return true, s.finish(ctx, r, x, state, errorCode, stored, response.Status, r.Input.Mapping)
	}
	return true, s.finish(ctx, r, x, "failed", "attempts_exhausted", nil, "", nil)
}
func (s *Service) liveTx(ctx context.Context, tx pgx.Tx, r Run, x Execution) (bool, error) {
	var live bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.id=$1 AND r.current_generation=$2 AND e.fence=$3 AND e.state='running' AND e.lease_until>clock_timestamp())`, r.ID, x.Generation, x.Fence).Scan(&live)
	return live, e
}
func (s *Service) finish(ctx context.Context, r Run, x Execution, state, code string, result *object, status string, mapping []Mapping) error {
	return s.workerTx(ctx, r, x.ActorID, func(tx pgx.Tx, _ k.Authorization, authorizationErr error) error {
		live, e := s.liveTx(ctx, tx, r, x)
		if e != nil || !live {
			return e
		}
		if authorizationErr != nil {
			state = "failed"
			code = "authorization_revoked"
			result = nil
			status = ""
		}
		m, e := methodTx(ctx, tx, r.Input.Entry.MethodID, r.Input.Entry.MethodVersion)
		if e != nil {
			return e
		}
		if m.State == "blocked" {
			state = "failed"
			code = "method_blocked"
			result = nil
			status = ""
		}
		return s.terminalTx(ctx, tx, r, x, state, code, result, status, mapping)
	})
}
func (s *Service) RunWorker(ctx context.Context, reporter diagnosticreport.Sink) error {
	group := diagnosticreport.NewGroup(reporter, "parsing", 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for {
				if ctx.Err() != nil {
					return
				}
				group.Begin(i, time.Now().Add(45*time.Second))
				worked, e := s.RunOne(ctx)
				reason := ""
				if e != nil {
					reason = "loop_failed"
				}
				group.End(i, reason, time.Now().Add(time.Second))
				if !worked || e != nil {
					timer := time.NewTimer(time.Second)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
		})
	}
	wg.Go(func() {
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				reporter.Begin("parsing-maintenance", time.Now().Add(30*time.Second))
				e := s.Cleanup(ctx)
				reason := ""
				if e != nil {
					reason = "loop_failed"
				}
				reporter.End("parsing-maintenance", reason, time.Now().Add(time.Minute))
			}
		}
	})
	<-ctx.Done()
	wg.Wait()
	return nil
}
