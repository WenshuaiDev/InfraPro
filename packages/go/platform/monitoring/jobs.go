package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Job struct {
	TraceID                       string          `json:"trace_id"`
	ResultState                   string          `json:"result_state,omitempty"`
	ID                            string          `json:"id"`
	Kind                          string          `json:"kind"`
	Target                        string          `json:"target_id"`
	TargetRevision                string          `json:"target_revision"`
	State                         string          `json:"state"`
	Revision                      string          `json:"revision"`
	Attempts                      int             `json:"attempts"`
	Progress                      json.RawMessage `json:"progress"`
	ErrorCode                     string          `json:"error_code"`
	CreatedAt                     time.Time       `json:"created_at"`
	UpdatedAt                     time.Time       `json:"updated_at"`
	tenant, project, actor, owner string
	fence                         int64
	input                         json.RawMessage
}
type JobPage struct {
	Items      []Job  `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func (j Job) scope() Scope {
	if j.project != "" {
		return ProjectScope(j.tenant, j.project)
	}
	return TenantScope(j.tenant)
}
func (j Job) action() string {
	switch j.Kind {
	case "validate", "publish":
		return "imports." + j.Kind
	case "export":
		return "exports.create"
	case "summary":
		var in summaryJobInput
		_ = json.Unmarshal(j.input, &in)
		if in.Mode == "query" {
			return in.Action
		}
	}
	return "details"
}
func workerActor(id string) access.Actor {
	return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}

const jobSelect = `SELECT id::text,kind,target_id::text,target_revision::text,state,revision::text,attempts,progress,COALESCE(error_code,''),created_at,updated_at,trace_id::text,tenant_id::text,COALESCE(project_id::text,''),COALESCE(actor_id::text,''),COALESCE(lease_owner,''),fence,input FROM monitoring_jobs`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	e := row.Scan(&j.ID, &j.Kind, &j.Target, &j.TargetRevision, &j.State, &j.Revision, &j.Attempts, &j.Progress, &j.ErrorCode, &j.CreatedAt, &j.UpdatedAt, &j.TraceID, &j.tenant, &j.project, &j.actor, &j.owner, &j.fence, &j.input)
	if e == pgx.ErrNoRows {
		return j, notFound()
	}
	return j, e
}
func assertJobTx(ctx context.Context, tx pgx.Tx, j Job) error {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT state='running' AND fence=$3 AND lease_owner=$4 AND lease_until>clock_timestamp() AND NOT cancel_requested FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, j.tenant, j.ID, j.fence, j.owner).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return problem("job_lease_lost", "任务租约已失效或任务已取消。", 409)
	}
	return nil
}

// RunOne is shared by the standalone worker and integration fault tests. A claim
// is durable; expired claims are fenced off before a replacement can execute.
func (s *Service) RunOne(ctx context.Context, owner string) (bool, error) {
	if e := s.requireStorage(); e != nil {
		return false, e
	}
	if e := s.reapExpiredJobs(ctx); e != nil {
		return false, e
	}
	var j Job
	claimed := false
	e := s.db(ctx, func(tx pgx.Tx) error {
		var id string
		e := tx.QueryRow(ctx, `SELECT id::text FROM monitoring_jobs WHERE kind IN ('validate','publish','summary','export','relocate','verify') AND ((SELECT mode FROM monitoring_environment_state WHERE id=1)='ready' OR actor_id IS NULL AND kind IN ('summary','verify')) AND attempts<5 AND NOT cancel_requested AND NOT (kind='summary' AND input->>'mode'='maintenance' AND EXISTS(SELECT 1 FROM monitoring_channel_versions v WHERE v.tenant_id=monitoring_jobs.tenant_id AND v.channel_id=target_id AND v.quarantined)) AND ((state IN ('queued','retry_wait') AND available_at<=clock_timestamp()) OR (state='running' AND lease_until<clock_timestamp())) ORDER BY CASE WHEN kind='summary' AND input->>'mode'='rebuild' THEN -1 WHEN kind IN ('validate','publish') THEN 0 WHEN actor_id IS NOT NULL THEN 1 ELSE 2 END,available_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='running',fence=fence+1,attempts=attempts+1,lease_owner=$2,lease_until=clock_timestamp()+interval '30 seconds',revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id, owner); e != nil {
			return e
		}
		j, e = scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, id))
		claimed = e == nil
		return e
	})
	if e != nil || !claimed {
		return claimed, e
	}
	work, cancel := context.WithCancel(ctx)
	work = TraceContext(work, j.TraceID)
	done := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-work.Done():
				return
			case <-ticker.C:
				e := s.jobTransaction(work, j, func(tx pgx.Tx) error {
					if e := assertJobTx(work, tx, j); e != nil {
						return e
					}
					_, e := tx.Exec(work, `UPDATE monitoring_jobs SET lease_until=clock_timestamp()+interval '30 seconds',updated_at=clock_timestamp() WHERE id=$1`, j.ID)
					return e
				})
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	switch j.Kind {
	case "validate":
		e = s.validateJob(work, j)
	case "publish":
		e = s.publishJob(work, j)
	case "summary":
		e = s.summaryJob(work, j)
	case "export":
		e = s.exportJob(work, j)
	case "relocate":
		e = s.relocateJob(work, j)
	case "verify":
		e = s.verifyJob(work, j)
	}
	LogOperation(work, "job."+j.Kind, map[bool]int{true: 500, false: 200}[e != nil], 0, errors.Is(e, context.Canceled))
	close(done)
	cancel()
	<-heartbeatDone
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	finish := s.finishJob(finishCtx, j, e)
	if finish != nil {
		return true, finish
	}
	return true, e
}

// Mechanical lease cleanup does not execute the former user's operation. It
// remains possible after their account is disabled and never publishes data.
func (s *Service) reapExpiredJobs(ctx context.Context) error {
	var candidates []Job
	e := s.db(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, jobSelect+` WHERE state='running' AND lease_until<clock_timestamp() AND (attempts>=5 OR cancel_requested) ORDER BY id LIMIT 50`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				return e
			}
			candidates = append(candidates, j)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, j := range candidates {
		e = s.db(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, false); e != nil {
				return e
			}
			if j.actor != "" {
				if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, j.actor); e != nil {
					return e
				}
			}
			if e := devicecoord.Catalog(ctx, tx, false); e != nil {
				return e
			}
			if e := devicecoord.Tenant(ctx, tx, j.tenant, true); e != nil {
				return e
			}
			result, e := tx.Exec(ctx, `UPDATE monitoring_jobs SET state=CASE WHEN cancel_requested THEN 'cancelled' ELSE 'failed' END,error_code='job_attempts_exhausted',lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND state='running' AND fence=$2 AND lease_until<clock_timestamp() AND (attempts>=5 OR cancel_requested)`, j.ID, j.fence)
			if e != nil || result.RowsAffected() == 0 {
				return e
			}
			if j.Kind == "publish" || j.Kind == "validate" {
				_, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='failed',error_code='job_attempts_exhausted',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3 AND state IN ('validating','preparing')`, j.tenant, j.Target, j.TargetRevision)
			}
			return e
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) RunWorker(ctx context.Context, logger *slog.Logger) error {
	owner := uuid.NewString()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	nextVerification := time.Time{}
	for {
		if time.Now().After(nextVerification) {
			nextVerification = time.Now().Add(time.Minute)
			if e := s.scheduleVerification(ctx); e != nil && ctx.Err() == nil {
				logger.Warn("monitoring reconciliation scheduling incomplete", "code", jobError(e))
			}
		}
		worked, e := s.RunOne(ctx, owner)
		if ctx.Err() != nil {
			return nil
		}
		if e != nil {
			logger.Warn("monitoring task incomplete", "code", jobError(e))
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func jobError(e error) string {
	if e == nil {
		return ""
	}
	var problem *Error
	if errors.As(e, &problem) {
		return problem.Code
	}
	var denial *access.Error
	if errors.As(e, &denial) {
		return "authorization_changed"
	}
	if errors.Is(e, context.Canceled) {
		return "job_interrupted"
	}
	return "monitoring_storage_unavailable"
}
func (s *Service) finishJob(ctx context.Context, j Job, cause error) error {
	return s.db(ctx, func(tx pgx.Tx) error {
		var state string
		var fence int64
		var cancelled bool
		e := tx.QueryRow(ctx, `SELECT state,fence,cancel_requested FROM monitoring_jobs WHERE id=$1 FOR UPDATE`, j.ID).Scan(&state, &fence, &cancelled)
		if e != nil {
			return e
		}
		if fence != j.fence || state != "running" {
			return nil
		}
		next, code := "succeeded", jobError(cause)
		if cancelled {
			next = "cancelled"
		} else if cause != nil {
			next = "failed"
			if code == "route_paused" || code == "route_changed" || code == "authorization_changed" || code == "content_changed" || code == "revision_changed" || code == "monitoring_not_found" || code == "resource_retired" || code == "snapshot_stale" || code == "snapshot_expired" || code == "export_invalidated" || code == "selection_stale" {
				next = "needs_action"
			} else if j.Attempts < 5 {
				next = "retry_wait"
			}
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state=$2,error_code=$3,lease_until=NULL,lease_owner=NULL,revision=revision+1,available_at=clock_timestamp()+($4::text||' seconds')::interval,updated_at=clock_timestamp() WHERE id=$1`, j.ID, next, nullable(code), strconv.Itoa(1<<min(j.Attempts, 5)))
		if e != nil {
			return e
		}
		if next == "failed" || next == "needs_action" {
			_, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='failed',error_code=$3,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$4 AND state IN ('validating','preparing')`, j.tenant, j.Target, code, j.TargetRevision)
		}
		return e
	})
}
func (s *Service) Jobs(ctx context.Context, a access.Actor, scope Scope, cursor string) (JobPage, error) {
	out := JobPage{Items: []Job{}}
	boundary, e := parseListCursor(cursor)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "jobs.list", false, false, func(tx pgx.Tx, auth authorization) error {
		rows, e := tx.Query(ctx, jobSelect+` WHERE tenant_id=$1 AND actor_id=$2 AND project_id IS NOT DISTINCT FROM $3::uuid AND ($4::timestamptz IS NULL OR (created_at,id)<($4::timestamptz,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT 51`, scope.Tenant, a.ID, nullable(scope.Project), boundary.At, nullable(boundary.ID))
		if e != nil {
			return e
		}
		var jobs []Job
		for rows.Next() {
			j, e := scanJob(rows)
			if e != nil {
				rows.Close()
				return e
			}
			jobs = append(jobs, j)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(jobs) > 50 {
			out.NextCursor = listCursor(jobs[49].CreatedAt, jobs[49].ID)
			jobs = jobs[:50]
		}
		for _, j := range jobs {
			if e := jobPermissionTx(ctx, tx, auth, j); e != nil {
				if isHidden(e) || errors.Is(e, access.ErrDenied) {
					continue
				}
				return e
			}
			if e = presentJobTx(ctx, tx, &j); e != nil {
				return e
			}
			out.Items = append(out.Items, j)
		}

		return nil
	})
	return out, e
}
func jobPermissionTx(ctx context.Context, tx pgx.Tx, auth authorization, j Job) error {
	if j.actor != auth.Account {
		return notFound()
	}
	if j.Kind == "validate" || j.Kind == "publish" {
		if _, e := batchTx(ctx, tx, auth, j.Target, false); e != nil {
			return e
		}
	}
	if j.Kind == "summary" {
		var input summaryJobInput
		if e := json.Unmarshal(j.input, &input); e != nil || input.Mode != "query" {
			return notFound()
		}
		var ids []string
		var revision string
		if e := tx.QueryRow(ctx, `SELECT sel.channel_ids::text[],sel.authorization_revision FROM monitoring_snapshots sn JOIN monitoring_selections sel ON sel.id=sn.selection_id WHERE sn.tenant_id=$1 AND sn.id=$2 AND sn.actor_id=$3 AND sel.project_id IS NOT DISTINCT FROM $4::uuid`, auth.Tenant, j.Target, auth.Account, nullable(auth.Project)).Scan(&ids, &revision); e != nil {
			if e == pgx.ErrNoRows {
				return notFound()
			}
			return e
		}
		if revision != authorizationVersion(auth) {
			return notFound()
		}
		if len(ids) > 0 {
			if _, e := authorizedWindowsTx(ctx, tx, auth, ids, "read"); e != nil {
				return e
			}
		}
	}
	if j.Kind == "export" {
		var input ExportInput
		if e := json.Unmarshal(j.input, &input); e != nil {
			return notFound()
		}
		var ids []string
		var revision string
		if e := tx.QueryRow(ctx, `SELECT sel.channel_ids::text[],sel.authorization_revision FROM monitoring_snapshots sn JOIN monitoring_selections sel ON sel.id=sn.selection_id WHERE sn.tenant_id=$1 AND sn.id=$2 AND sn.actor_id=$3 AND sel.project_id IS NOT DISTINCT FROM $4::uuid`, auth.Tenant, input.Query.Snapshot, auth.Account, nullable(auth.Project)).Scan(&ids, &revision); e != nil {
			if e == pgx.ErrNoRows {
				return notFound()
			}
			return e
		}
		if revision != authorizationVersion(auth) {
			return notFound()
		}
		if len(ids) > 0 {
			if _, e := authorizedWindowsTx(ctx, tx, auth, ids, "export"); e != nil {
				return e
			}
			if _, e := authorizedWindowsTx(ctx, tx, auth, ids, "read"); e != nil {
				return e
			}
		}
	}
	if auth.Entry == "project" {
		_, e := access.RequireProjectTx(ctx, tx, auth.Account, access.MonitoringOperation("project", j.action()), auth.Tenant, auth.Project)
		return e
	}
	return access.RequireScopedTx(ctx, tx, auth.Account, access.MonitoringOperation("tenant", j.action()), auth.Tenant)
}
func (s *Service) Job(ctx context.Context, a access.Actor, scope Scope, id string) (Job, error) {
	var out Job
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "jobs.detail", false, false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2 AND project_id IS NOT DISTINCT FROM $3::uuid`, scope.Tenant, id, nullable(scope.Project)))
		if e != nil {
			return e
		}
		if e = jobPermissionTx(ctx, tx, auth, out); e != nil {
			return e
		}
		return presentJobTx(ctx, tx, &out)
	})
	return out, e
}
func (s *Service) CancelImport(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta) (ImportBatch, error) {
	out := ImportBatch{}
	if e := in.validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "imports.cancel", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "imports.cancel:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		b, e := batchTx(ctx, tx, auth, id, true)
		if e != nil {
			return e
		}
		if b.Revision != in.ExpectedRevision || b.State == "published" || b.State == "revoked" || b.State == "completed_noop" {
			return revisionChanged()
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET cancel_requested=true,state=CASE WHEN state IN ('queued','retry_wait') THEN 'cancelled' ELSE state END,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND target_id=$2 AND state IN ('queued','retry_wait','running')`, scope.Tenant, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='cancelled',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id); e != nil {
			return e
		}
		out, e = batchTx(ctx, tx, auth, id, false)
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "imports.cancel:"+id, in.RequestID, in, out)
	})
	return out, e
}
func (s *Service) CancelJob(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta) (Job, error) {
	out := Job{}
	if e := in.validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "jobs.cancel", false, true, func(tx pgx.Tx, auth authorization) error {
		hit, e := receiptTx(ctx, tx, auth, "jobs.cancel:"+id, in.RequestID, in, &out)
		if e != nil || hit {
			return e
		}
		j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, scope.Tenant, id))
		if e != nil {
			return e
		}
		if e = jobPermissionTx(ctx, tx, auth, j); e != nil {
			return e
		}
		if j.Revision != in.ExpectedRevision {
			return revisionChanged()
		}
		if j.State != "queued" && j.State != "retry_wait" && j.State != "running" {
			return revisionChanged()
		}
		if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET cancel_requested=true,state=CASE WHEN state='running' THEN state ELSE 'cancelled' END,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, id); e != nil {
			return e
		}
		if j.Kind == "validate" || j.Kind == "publish" {
			if _, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='cancelled',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3 AND state IN ('validating','preparing')`, scope.Tenant, j.Target, j.TargetRevision); e != nil {
				return e
			}
		}
		out, e = scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, id))
		if e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "jobs.cancel:"+id, in.RequestID, in, out)
	})
	return out, e
}
func countText(n int64) string { return strconv.FormatInt(n, 10) }

func presentJobTx(ctx context.Context, tx pgx.Tx, j *Job) error {
	if j.Kind != "export" {
		return nil
	}
	var expired, invalidated bool
	if e := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp(),invalidated_at IS NOT NULL FROM monitoring_exports WHERE tenant_id=$1 AND job_id=$2`, j.tenant, j.ID).Scan(&expired, &invalidated); e != nil {
		return e
	}
	if invalidated {
		j.ResultState = "invalidated"
	} else if expired {
		j.ResultState = "expired"
	} else if j.State == "succeeded" {
		j.ResultState = "available"
	} else {
		j.ResultState = "preparing"
	}
	if expired || invalidated {
		j.Progress = encodeJSON(map[string]string{"phase": j.ResultState})
	}
	return nil
}
