package parsing

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func publishTaskTx(ctx context.Context, tx pgx.Tx, id, event, actor string) (taskrecord.Ref, error) {
	var c taskrecord.Change
	var rawInput []byte
	var tenant *string
	var object *string
	var generation, revision, control, fence int64
	e := tx.QueryRow(ctx, `SELECT r.id::text,r.tenant_id::text,r.created_by::text,r.current_generation,r.revision,r.control_revision,r.input_snapshot,r.created_at,e.actor_id::text,e.state,e.fence,e.error_code,e.result_object_id::text,clock_timestamp(),(SELECT count(*) FROM parsing_run_attempts a WHERE a.run_id=r.id AND a.generation=r.current_generation) FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.id=$1`, id).Scan(&c.RootID, &tenant, &c.Originator, &generation, &revision, &control, &rawInput, &c.CreatedAt, &c.RequestedBy, &c.State, &fence, &c.ErrorCode, &object, &c.ObservedAt, &c.Attempt)
	if e != nil {
		return taskrecord.Ref{}, e
	}
	c.Module = "parsing"
	c.Kind = "run"
	c.Type = "parsing.test"
	c.Scope = taskrecord.Scope{Kind: "platform"}
	if tenant != nil {
		c.Scope = taskrecord.Scope{Kind: "tenant", Tenant: *tenant}
	}
	c.SourceState = c.State
	if c.State == "canceled" {
		c.State = "cancelled"
	}
	c.Phase = c.State
	if c.State == "succeeded" {
		c.Phase = "completed"
	}
	if c.State == "running" {
		c.Phase = "computing"
	}
	c.Generation = strconv.FormatInt(generation, 10)
	c.Revision = strconv.FormatInt(revision, 10)
	c.ControlRevision = strconv.FormatInt(control, 10)
	c.Fence = strconv.FormatInt(fence, 10)
	c.OriginKind = "person"
	c.EventActor = actor
	c.ExecutorKind = "worker"
	c.ExecutorRef = "monitoring-worker/parsing"
	c.Event = event
	c.InputHash = taskrecord.Hash(rawInput)
	c.InputRef = map[string]string{"job_id": id, "generation": c.Generation}
	c.ResultRef = map[string]string{}
	if object != nil {
		c.ResultRef = map[string]string{"job_id": id, "generation": c.Generation}
	}
	if _, e = tx.Exec(ctx, `UPDATE parsing_runs SET task_snapshot=$2 WHERE id=$1`, id, k.JSON(c)); e != nil {
		return taskrecord.Ref{}, e
	}
	return taskrecord.PublishTx(ctx, tx, c)
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	out := []tasks.View{}
	for _, ref := range refs {
		if ref.Module != "parsing" || ref.Kind != "run" {
			continue
		}
		var fact taskrecord.Change
		e := tx.QueryRow(ctx, `SELECT task_snapshot FROM parsing_runs WHERE id=$1`, ref.RootID).Scan(&fact)
		if e == pgx.ErrNoRows {
			continue
		}
		if e != nil {
			return nil, e
		}
		scope := platform()
		if fact.Scope.Kind == "tenant" {
			scope = tenant(fact.Scope.Tenant)
		}
		auth, e := k.AuthorizeTx(ctx, tx, a, scope, op(scope, "runs", "detail"))
		if denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		r, e := runTx(ctx, tx, scope, ref.RootID)
		if e != nil {
			return nil, e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, false); denied(e) {
			continue
		} else if e != nil {
			return nil, e
		}
		canExecute := s.authorizeRunTx(ctx, tx, auth, r, true) == nil
		if mode == "managed" && !canExecute || mode == "mine" && r.CreatedBy != a.ID {
			continue
		}
		v := tasks.Present(fact, "平台")
		if scope.Entry == "tenant" {
			v.ScopeLabel = "租户"
		}
		v.ActorLabel = "其他成员"
		if r.CreatedBy == a.ID {
			v.ActorLabel = "我"
		}
		v.ResultReadable = true
		m, e := methodTx(ctx, tx, r.Input.Entry.MethodID, r.Input.Entry.MethodVersion)
		if e != nil {
			return nil, e
		}
		tasks.Capability(&v, "cancel", canExecute && taskrecord.Active(v.State), true, "task_not_active")
		tasks.Capability(&v, "retry", canExecute && s.options.Enabled && (v.State == "failed" || v.State == "cancelled") && m.State != "blocked", true, "method_unavailable")
		v.NextStep = "view_parsing_evidence"
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, in tasks.Command) (tasks.CommandResult, error) {
	var out tasks.CommandResult
	if e := in.Validate(); e != nil {
		return out, e
	}
	if ref.Module != "parsing" || ref.Kind != "run" {
		return out, tasks.Hidden()
	}
	var scope Scope
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var tid *string
		if e := tx.QueryRow(ctx, `SELECT tenant_id::text FROM parsing_runs WHERE id=$1`, ref.RootID).Scan(&tid); e != nil {
			return tasks.Hidden()
		}
		scope = platform()
		if tid != nil {
			scope = tenant(*tid)
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	e = k.Transact(ctx, s.access, a, scope, "TasksCommand", k.Locks{CatalogWrite: scope.Entry == "platform", TenantWrite: scope.Entry == "tenant"}, func(tx pgx.Tx, auth k.Authorization) error {
		r, e := runTx(ctx, tx, scope, ref.RootID)
		if e != nil {
			return e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, true); e != nil {
			return e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, false); e != nil {
			return e
		}
		meta := Meta{RequestID: in.RequestID, Reason: in.Reason}
		if meta.Reason == "" {
			meta.Reason = "取消本人试解析"
		}
		operation := "runs." + in.Action
		payload := []any{ref.RootID, in}
		v, ok, e := replay(ctx, tx, auth, operation, meta, payload)
		if e != nil {
			return e
		}
		if ok {
			if v.State == "canceled" {
				v.State = "cancelled"
			}
			out = tasks.CommandResult{TaskID: ref.ID(), Generation: v.Generation, State: v.State, CommandState: "applied", Replayed: true}
			return nil
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
		generation := r.Generation
		state := "canceled"
		if in.Action == "cancel" {
			if _, e = tx.Exec(ctx, `UPDATE parsing_run_executions SET state='canceled',fence=fence+1,lease_until=NULL,finished_at=clock_timestamp(),error_code='canceled' WHERE run_id=$1 AND generation=$2 AND state IN ('queued','running')`, r.ID, r.Generation); e != nil {
				return e
			}
		} else {
			var queued int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1 AND e.state='queued'`, scopeKey(scope)).Scan(&queued); e != nil {
				return e
			}
			if queued >= s.options.Queue {
				return k.Problem("queue_budget_exceeded", "当前范围队列已满。", 429)
			}
			g, _ := strconv.ParseInt(r.Generation, 10, 64)
			generation = strconv.FormatInt(g+1, 10)
			state = "queued"
			if _, e = tx.Exec(ctx, `INSERT INTO parsing_run_executions(run_id,generation,actor_id,state) VALUES($1,$2,$3,'queued')`, r.ID, generation, a.ID); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE parsing_runs SET current_generation=$2,revision=revision+1,control_revision=control_revision+1 WHERE id=$1`, r.ID, generation); e != nil {
			return e
		}
		ref, e := publishTaskTx(ctx, tx, r.ID, in.Action, a.ID)
		if e != nil {
			return e
		}
		result := Result{ID: r.ID, Generation: generation, State: state, TaskRef: &ref}
		if e = complete(ctx, tx, auth, operation, meta, payload, result, nil, map[string]string{"run_id": r.ID, "generation": generation, "state": state}); e != nil {
			return e
		}
		if state == "canceled" {
			state = "cancelled"
		}
		out = tasks.CommandResult{TaskID: ref.TaskID, Generation: generation, State: state, CommandState: "applied"}
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
	x, e := executionTx(ctx, tx, ref.RootID, generation)
	if e != nil {
		return tasks.Target{}, e
	}
	out := tasks.Target{Available: true, Kind: "parsing_run", Scope: views[0].Scope, ID: ref.RootID, Generation: generation}
	if kind == "result" && x.ResultObjectID == nil {
		out.Available = false
		out.Reason = "result_unavailable"
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "parsing", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		key := "platform"
		if scope.Kind == "tenant" {
			key = "tenant:" + scope.Tenant
		}
		var q, r, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE e.state='queued')::text,count(*) FILTER(WHERE e.state='running')::text,count(*) FILTER(WHERE e.state='failed')::text FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE r.scope_key=$1`, key).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}

type TaskIndex struct{}

func (TaskIndex) BackfillTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]taskrecord.Change, string, bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,snapshot FROM parsing_task_metadata WHERE ($1::uuid IS NULL OR id>$1) ORDER BY id LIMIT $2`, k.Nullable(after), limit)
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
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parsing_task_metadata v LEFT JOIN task_records r ON r.source_module='parsing' AND r.source_kind='run' AND r.source_root_id=v.id WHERE r.id IS NULL OR r.source_revision<v.revision)`).Scan(&missing)
	return missing, e
}
