package displayread

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) checkQueueBudgetTx(ctx context.Context, tx pgx.Tx, o Owner) error {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('display-read:'||$1,0))`, o.Tenant); e != nil {
		return e
	}
	var tenant, project, instance int
	e := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE project_id=$2),count(*) FILTER(WHERE instance_id=$3) FROM display_read_queries WHERE tenant_id=$1 AND state IN ('queued','running') AND expires_at>clock_timestamp()`, o.Tenant, o.Project, o.Instance).Scan(&tenant, &project, &instance)
	if e != nil {
		return e
	}
	if tenant >= 128 || project >= 32 || instance >= 4 {
		return Problem("budget_exceeded", "后台查询并发已达上限，请等待现有查询完成。", 429)
	}
	return nil
}

type readJob struct {
	id, worker string
	fence      int64
	deadline   time.Time
	plan       Plan
	work       json.RawMessage
}

// ProcessOne shares the monitoring worker process, while its durable lease only
// owns read coordination. Monitoring summary jobs retain their own queue/fence.
func (s *Service) ProcessOne(ctx context.Context, worker string) (bool, error) {
	if s.authority == nil {
		return false, nil
	}
	j := readJob{worker: worker}
	claimed := false
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		// Serialize the short claim transaction so concurrent workers cannot
		// both observe one available slot and exceed the per-instance budget.
		var claimLock bool
		if e := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('display-read-claim',0))`).Scan(&claimLock); e != nil {
			return e
		}
		if !claimLock {
			return nil
		}
		if _, e := tx.Exec(ctx, `WITH expired AS(UPDATE display_read_jobs SET state='failed',fence=fence+1,lease_until=NULL WHERE state IN ('queued','running') AND deadline<=clock_timestamp() RETURNING query_id) UPDATE display_read_queries SET state='failed',error_code='query_timeout' WHERE id IN(SELECT query_id FROM expired)`); e != nil {
			return e
		}
		// Prefer projects with fewer running jobs, then oldest work; SKIP LOCKED
		// prevents competing workers from occupying the same request.
		e := tx.QueryRow(ctx, `SELECT j.query_id::text FROM display_read_jobs j JOIN display_read_queries q ON q.id=j.query_id
 WHERE j.deadline>clock_timestamp() AND j.available_at<=clock_timestamp() AND (j.state='queued' OR j.state='running' AND j.lease_until<=clock_timestamp()) AND q.state IN ('queued','running') AND q.expires_at>clock_timestamp() AND (SELECT count(*) FROM display_read_jobs busy JOIN display_read_queries bq ON bq.id=busy.query_id WHERE busy.state='running' AND busy.lease_until>clock_timestamp() AND bq.instance_id=q.instance_id)<2
 ORDER BY (SELECT count(*) FROM display_read_jobs active JOIN display_read_queries aq ON aq.id=active.query_id WHERE active.state='running' AND active.lease_until>clock_timestamp() AND aq.project_id=q.project_id),j.created_at,j.query_id FOR UPDATE OF j SKIP LOCKED LIMIT 1`).Scan(&j.id)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `UPDATE display_read_jobs SET state='running',worker=$2,fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+interval '12 seconds',first_started_at=COALESCE(first_started_at,clock_timestamp()),updated_at=clock_timestamp() WHERE query_id=$1 RETURNING fence,deadline`, j.id, worker).Scan(&j.fence, &j.deadline); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET state='running' WHERE id=$1`, j.id); e != nil {
			return e
		}
		claimed = true
		return nil
	})
	if e != nil || !claimed {
		return claimed, e
	}
	started := time.Now()
	workCtx, cancel := context.WithDeadline(ctx, j.deadline)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-tick.C:
				err := s.access.Transact(workCtx, func(tx pgx.Tx) error {
					tag, e := tx.Exec(workCtx, `UPDATE display_read_jobs SET lease_until=clock_timestamp()+interval '12 seconds',updated_at=clock_timestamp() WHERE query_id=$1 AND worker=$2 AND fence=$3 AND state='running' AND lease_until>clock_timestamp()`, j.id, worker, j.fence)
					if e != nil {
						return e
					}
					if tag.RowsAffected() != 1 {
						return errors.New("lease lost")
					}
					return nil
				})
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	data, runErr := s.executeJob(workCtx, &j)
	cancel()
	wg.Wait()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	runErr = ReadError(runErr)
	finishErr := s.finishJob(finishCtx, j, data, runErr)
	slog.Info("display read", "query_id", j.id, "method", j.plan.Method, "duration_ms", time.Since(started).Milliseconds(), "pending", len(data.Work) > 0, "failed", runErr != nil)
	return true, finishErr
}
func (s *Service) executeJob(ctx context.Context, j *readJob) (Materialized, error) {
	var o Owner
	var raw, proof json.RawMessage
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant_id::text,project_id::text,instance_id::text,subject_id::text,session_id::text,config_id::text,environment::text,method,request,scopes,source_vector,work,proof,id::text,read_snapshot_id::text,metadata_as_of,created_at,expires_at,scope_hash,filter_hash FROM display_read_queries WHERE id=$1 AND state='running'`, j.id).Scan(&o.Tenant, &o.Project, &o.Instance, &o.Subject, &o.Session, &o.Config, &o.Environment, &j.plan.Method, &raw, &j.plan.Scopes, &j.plan.Evidence, &j.work, &proof, &j.plan.Query.ID, &j.plan.Query.SnapshotID, &j.plan.Query.MetadataAsOf, &j.plan.Query.CreatedAt, &j.plan.Query.ExpiresAt, &j.plan.Query.ScopeHash, &j.plan.Query.FilterHash)
	})
	if e != nil {
		return Materialized{}, e
	}
	j.plan.Query.Method = j.plan.Method
	j.plan.Request, e = DecodeRequest(j.plan.Method, raw)
	if e != nil {
		return Materialized{}, e
	}
	j.plan.Authority, e = s.authority(ctx, o)
	if e != nil {
		return Materialized{}, e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := j.plan.Authority.Validate(ctx, tx, j.plan.Method, &j.plan.Evidence); e != nil {
			return e
		}
		if validate := s.validators[j.plan.Method]; validate != nil {
			return validate(ctx, tx, j.plan, proof)
		}
		return nil
	})
	if e != nil {
		return Materialized{}, e
	}
	run := s.executors[j.plan.Method]
	if run == nil {
		return Materialized{}, Problem("capability_unavailable", "后台读取实现未安装。", 409)
	}
	return run(ctx, j.plan, j.work)
}
func (s *Service) finishJob(ctx context.Context, j readJob, data Materialized, runErr error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var valid bool
		e := tx.QueryRow(ctx, `SELECT state='running' AND worker=$2 AND fence=$3 AND lease_until>clock_timestamp() FROM display_read_jobs WHERE query_id=$1 FOR UPDATE`, j.id, j.worker, j.fence).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return nil
		}
		if runErr == nil {
			_, runErr = j.plan.Authority.Validate(ctx, tx, j.plan.Method, &j.plan.Evidence)
		}
		if runErr == nil {
			runErr = s.checkParentTx(ctx, tx, j.id)
		}
		if runErr == nil && len(data.Work) == 0 {
			if validate := s.validators[j.plan.Method]; validate != nil {
				runErr = validate(ctx, tx, j.plan, data.Proof)
			}
		}
		if runErr != nil {
			code := "query_failed"
			if errors.Is(runErr, context.DeadlineExceeded) {
				code = "query_timeout"
			} else {
				var problem struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(Bytes(runErr), &problem)
				if problem.Code != "" {
					code = problem.Code
				}
			}
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET state='failed',error_code=$2 WHERE id=$1`, j.id, code); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE display_read_jobs SET state='failed',lease_until=NULL WHERE query_id=$1`, j.id)
			return e
		}
		if len(data.Work) > 0 {
			if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET state='queued',work=$2 WHERE id=$1`, j.id, data.Work); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE display_read_jobs SET state='queued',worker=NULL,lease_until=NULL,available_at=clock_timestamp()+interval '300 milliseconds',updated_at=clock_timestamp() WHERE query_id=$1`, j.id)
			return e
		}
		state := data.State
		if state == "" {
			state = "ready"
			if len(data.Rows) == 0 && data.Single == nil && !isStatistics(j.plan.Method) {
				state = "empty"
			}
		}
		if data.Metadata == nil {
			data.Metadata = map[string]any{}
		}
		if data.Single != nil {
			data.Metadata["single"] = data.Single
		}
		if data.Completeness.Scope == "" {
			data.Completeness = Completeness{Scope: "complete", Data: "complete", Reasons: []string{}}
		}
		if _, e = tx.Exec(ctx, `DELETE FROM display_read_query_rows WHERE query_id=$1`, j.id); e != nil {
			return e
		}
		if _, e = tx.CopyFrom(ctx, pgx.Identifier{"display_read_query_rows"}, []string{"query_id", "ordinal", "sort_key", "data"}, pgx.CopyFromSlice(len(data.Rows), func(i int) ([]any, error) { return []any{j.id, i, strconv.Itoa(i), data.Rows[i]}, nil })); e != nil {
			return e
		}
		if e = s.saveSectionsTx(ctx, tx, j.id, data.Sections); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE display_read_queries SET state=$2,metadata=$3,proof=COALESCE($4,proof),row_count=$5,completeness=$6,result_bytes=$7,work=NULL,completed_at=clock_timestamp() WHERE id=$1`, j.id, state, Bytes(data.Metadata), data.Proof, len(data.Rows), Bytes(data.Completeness), len(Bytes(data.Rows))+len(Bytes(data.Metadata))); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE display_read_jobs SET state='done',lease_until=NULL,updated_at=clock_timestamp() WHERE query_id=$1`, j.id)
		return e
	})
}
func (s *Service) Cleanup(ctx context.Context) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `DELETE FROM display_read_queries WHERE id IN(SELECT q.id FROM display_read_queries q JOIN display_environment env ON env.id=1 LEFT JOIN display_sessions s ON s.id=q.session_id WHERE q.expires_at<=clock_timestamp() OR q.environment<>env.generation OR s.state IN ('closed','revoked','expired') ORDER BY q.expires_at LIMIT 1000)`)
		return e
	})
}
func (s *Service) RunWorker(ctx context.Context, owner string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	cleanup := time.NewTicker(time.Minute)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-cleanup.C:
			if e := s.Cleanup(ctx); e != nil {
				return e
			}
		case <-ticker.C:
			if _, e := s.ProcessOne(ctx, owner); e != nil && ctx.Err() == nil {
				slog.Warn("display read worker retry", "error", e)
			}
		}
	}
}
