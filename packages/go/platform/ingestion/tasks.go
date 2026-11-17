package ingestion

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

func publishTaskTx(ctx context.Context, tx pgx.Tx, id, event, actor string) (taskrecord.Ref, error) {
	var fact taskrecord.Change
	var tenant *string
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT id::text,tenant_id::text,actor_id::text,generation::text,revision::text,state,input,created_at,clock_timestamp() FROM ingestion_recovery_jobs WHERE id=$1`, id).Scan(&fact.RootID, &tenant, &fact.Originator, &fact.Generation, &fact.Revision, &fact.State, &raw, &fact.CreatedAt, &fact.ObservedAt)
	if e != nil {
		return taskrecord.Ref{}, e
	}
	fact.Module = "ingestion"
	fact.Kind = "recovery"
	fact.Type = "ingestion.reprocess"
	fact.Scope = taskrecord.Scope{Kind: "platform"}
	if tenant != nil {
		fact.Scope = taskrecord.Scope{Kind: "tenant", Tenant: *tenant}
	}
	fact.SourceState = fact.State
	fact.Phase = fact.State
	if fact.State == "running" {
		fact.Phase = "computing"
	}
	if fact.State == "succeeded" {
		fact.Phase = "completed"
	}
	fact.ControlRevision = fact.Revision
	fact.Fence = fact.Generation
	fact.RequestedBy = fact.Originator
	fact.OriginKind = "person"
	fact.ExecutorKind = "worker"
	fact.ExecutorRef = "ingest-worker/recovery"
	fact.EventActor = actor
	fact.Event = event
	fact.InputHash = taskrecord.Hash(raw)
	fact.InputRef = map[string]string{"job_id": id, "generation": fact.Generation}
	fact.ResultRef = map[string]string{}
	if fact.State == "succeeded" || fact.State == "failed" {
		fact.ResultRef = map[string]string{"job_id": id, "generation": fact.Generation}
	}
	if _, e = tx.Exec(ctx, `UPDATE ingestion_recovery_jobs SET task_snapshot=$2 WHERE id=$1`, id, k.JSON(fact)); e != nil {
		return taskrecord.Ref{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO ingestion_recovery_generations(job_id,generation,snapshot) SELECT id,generation,`+recoveryJSON+` FROM ingestion_recovery_jobs j WHERE id=$1 ON CONFLICT(job_id,generation) DO UPDATE SET snapshot=EXCLUDED.snapshot`, id); e != nil {
		return taskrecord.Ref{}, e
	}
	return taskrecord.PublishTx(ctx, tx, fact)
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	out := []tasks.View{}
	for _, ref := range refs {
		if ref.Module != "ingestion" || ref.Kind != "recovery" {
			continue
		}
		job, e := recoveryTx(ctx, tx, ref.RootID)
		if e != nil {
			continue
		}
		scope := recoveryScope(job)
		auth, e := k.AuthorizeTx(ctx, tx, a, scope, op(scope, "recovery", "list"))
		if e != nil {
			continue
		}
		if _, e = s.authorizeRecoveryTx(ctx, tx, auth, job.Input); e != nil {
			continue
		}
		if mode == "mine" && job.ActorID != a.ID {
			continue
		}
		var fact taskrecord.Change
		if e = tx.QueryRow(ctx, `SELECT task_snapshot FROM ingestion_recovery_jobs WHERE id=$1`, job.ID).Scan(&fact); e != nil {
			return nil, e
		}
		v := tasks.Present(fact, "平台")
		if scope.Entry == "tenant" {
			v.ScopeLabel = "租户"
		}
		v.ActorLabel = "其他成员"
		if a.ID == job.ActorID {
			v.ActorLabel = "我"
		}
		v.ResultReadable = true
		tasks.Capability(&v, "cancel", taskrecord.Active(v.State), true, "task_not_active")
		tasks.Capability(&v, "retry", s.options.Enabled && (v.State == "failed" || v.State == "cancelled"), true, "task_not_retryable")
		v.NextStep = "view_ingestion_recovery"
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, in tasks.Command) (tasks.CommandResult, error) {
	var out tasks.CommandResult
	if e := in.Validate(); e != nil {
		return out, e
	}
	var job Recovery
	e := s.access.Transact(ctx, func(tx pgx.Tx) error { var e error; job, e = recoveryTx(ctx, tx, ref.RootID); return e })
	if e != nil {
		return out, e
	}
	scope := recoveryScope(job)
	e = k.Transact(ctx, s.access, a, scope, "TasksCommand", k.Locks{CatalogWrite: scope.Entry == "platform", TenantWrite: scope.Entry == "tenant"}, func(tx pgx.Tx, auth k.Authorization) error {
		job, e = recoveryTx(ctx, tx, ref.RootID)
		if e != nil {
			return e
		}
		if _, e = s.authorizeRecoveryTx(ctx, tx, auth, job.Input); e != nil {
			return e
		}
		m := Meta{RequestID: in.RequestID, Reason: in.Reason}
		if m.Reason == "" {
			m.Reason = "接入恢复任务控制"
		}
		payload := []any{ref.RootID, in}
		if previous, ok, e := replay(ctx, tx, auth, "recovery."+in.Action, m, payload); e != nil || ok {
			out = tasks.CommandResult{TaskID: ref.ID(), Generation: previous.Revision, State: previous.State, CommandState: "applied", Replayed: ok}
			return e
		}
		views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		if e = tasks.ValidateCommand(in, views[0], a); e != nil {
			return e
		}
		if e = tasks.SetCommandTx(ctx, tx, a, in, views[0]); e != nil {
			return e
		}
		state, generation := "cancelled", job.Generation
		if in.Action == "retry" {
			state = "queued"
			generation = numstr(num(generation) + 1)
		}
		if _, e = tx.Exec(ctx, `UPDATE ingestion_recovery_jobs SET state=$2,generation=$3,revision=revision+1,actor_id=$4,updated_at=clock_timestamp() WHERE id=$1`, job.ID, state, generation, a.ID); e != nil {
			return e
		}
		if in.Action == "cancel" {
			if _, e = tx.Exec(ctx, `UPDATE ingestion_identity_decisions SET state='blocked',fence=fence+1,lease_until=NULL,error_code='recovery_cancelled' WHERE recovery_id=$1 AND state IN ('queued','retry_wait','identifying')`, job.ID); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE ingestion_parse_plans SET state='blocked',fence=fence+1,lease_until=NULL,error_code='recovery_cancelled',finished_at=clock_timestamp() WHERE snapshot->>'recovery_id'=$1 AND state IN ('queued','retry_wait','parsing')`, job.ID); e != nil {
				return e
			}
		}
		task, e := publishTaskTx(ctx, tx, job.ID, in.Action, a.ID)
		if e != nil {
			return e
		}
		result := Result{ID: job.ID, Revision: generation, State: state, TaskRef: &task}
		if e = complete(ctx, tx, auth, "recovery."+in.Action, m, payload, result, nil, map[string]string{"state": state, "generation": generation}); e != nil {
			return e
		}
		out = tasks.CommandResult{TaskID: task.TaskID, Generation: generation, State: state, CommandState: "applied"}
		return nil
	})
	return out, e
}
func (s *Service) TaskTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation, kind string) (tasks.Target, error) {
	views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
	if e != nil {
		return tasks.Target{}, e
	}
	if len(views) != 1 {
		return tasks.Target{}, tasks.Hidden()
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_executions WHERE task_id=$1 AND generation=$2)`, ref.ID(), generation).Scan(&exists); e != nil {
		return tasks.Target{}, e
	}
	if !exists {
		return tasks.Target{}, tasks.Hidden()
	}
	return tasks.Target{Available: true, Kind: "ingestion_recovery", Scope: views[0].Scope, ID: ref.RootID, Generation: generation}, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "ingestion", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var queued, running, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='queued')::text,count(*) FILTER(WHERE state='running')::text,count(*) FILTER(WHERE state='failed')::text FROM ingestion_recovery_jobs WHERE tenant_id IS NOT DISTINCT FROM $1::uuid`, k.Nullable(scope.Tenant)).Scan(&queued, &running, &failed)
		return queued, running, failed, e
	})
}

type TaskIndex struct{}

func (TaskIndex) BackfillTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]taskrecord.Change, string, bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,snapshot FROM ingestion_task_metadata WHERE ($1::uuid IS NULL OR id>$1) ORDER BY id LIMIT $2`, k.Nullable(after), limit)
	if e != nil {
		return nil, "", false, e
	}
	defer rows.Close()
	out := []taskrecord.Change{}
	next := after
	for rows.Next() {
		var c taskrecord.Change
		if e = rows.Scan(&next, &c); e != nil {
			return nil, "", false, e
		}
		out = append(out, c)
	}
	return out, next, len(out) < limit, rows.Err()
}
func (TaskIndex) MissingTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var missing bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingestion_task_metadata v LEFT JOIN task_records r ON r.source_module='ingestion' AND r.source_kind='recovery' AND r.source_root_id=v.id WHERE r.id IS NULL OR r.source_revision<v.revision)`).Scan(&missing)
	return missing, e
}
