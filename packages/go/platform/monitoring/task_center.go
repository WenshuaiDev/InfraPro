package monitoring

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"slices"
)

// TaskSummaryAuthorizer is implemented by the calculation owner. Management
// disclosure is separate from its complete run/input/result reader.
type automaticRunCounter interface {
	TaskAutomaticCountsTx(context.Context, pgx.Tx, access.Actor, taskrecord.Scope) (string, string, string, error)
}

type runResultResolver interface {
	TaskRunResultTx(context.Context, pgx.Tx, access.Actor, taskrecord.Scope, string, string) (string, string, map[string]string, error)
}

type TaskSummaryAuthorizer interface {
	AuthorizeTaskSummaryTx(context.Context, pgx.Tx, Task, access.Actor, bool) error
}

func taskHidden(e error) bool {
	var ae *access.Error
	var me *Error
	return tasks.Denied(e) || e == pgx.ErrNoRows || errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404) || errors.As(e, &me) && (me.Status == 403 || me.Status == 404)
}
func (s *Service) taskAuthTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) (authorization, error) {
	auth := authorization{Scope: scope, Account: a.ID, ResourceRevision: "0", DataRevision: "0"}
	e := tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&auth.Revision)
	if e != nil {
		return auth, e
	}
	if scope.Project != "" {
		e = tx.QueryRow(ctx, `SELECT r.revision::text,COALESCE(m.revision,0)::text FROM project_resource_policies r LEFT JOIN monitoring_access_states m USING(tenant_id,project_id) WHERE r.tenant_id=$1 AND r.project_id=$2`, scope.Tenant, scope.Project).Scan(&auth.ResourceRevision, &auth.DataRevision)
	}
	return auth, e
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	out := []tasks.View{}
	ids := []string{}
	wanted := map[string]bool{}
	for _, r := range refs {
		ids = append(ids, r.RootID)
		wanted[r.Key()] = true
	}
	rows, e := tx.Query(ctx, `SELECT snapshot FROM monitoring_task_metadata WHERE snapshot IS NOT NULL AND (snapshot->>'source_root_id'=ANY($1::text[]) OR id::text=ANY($1::text[]))`, ids)
	if e != nil {
		return nil, e
	}
	facts := []taskrecord.Change{}
	for rows.Next() {
		var c taskrecord.Change
		if e = rows.Scan(&c); e != nil {
			rows.Close()
			return nil, e
		}
		if wanted[c.SourceRef.Key()] || wanted["monitoring:job:"+c.InputRef["job_id"]] {
			facts = append(facts, c)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	type taskScope struct {
		codes         []access.Code
		label, status string
		auth          authorization
	}
	// Qualification is cached only inside this actor's current read transaction,
	// while the authorization graph lock is held. Result/input checks stay per job.
	scopes := map[taskrecord.Scope]taskScope{}
	for _, c := range facts {
		cached, ok := scopes[c.Scope]
		if !ok {
			codes, label, status, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
			if taskHidden(e) {
				continue
			}
			if e != nil {
				return nil, e
			}
			auth, e := s.taskAuthTx(ctx, tx, a, Scope{Entry: c.Scope.Kind, Tenant: c.Scope.Tenant, Project: c.Scope.Project})
			if e != nil {
				return nil, e
			}
			cached = taskScope{codes, label, status, auth}
			scopes[c.Scope] = cached
		}
		codes, label, status, auth := cached.codes, cached.label, cached.status, cached.auth
		j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, c.InputRef["job_id"]))
		if e != nil {
			return nil, e
		}
		d, _ := taskrecord.Lookup(c.Type)
		manager := tasks.CanManage(codes, d, c.Scope.Kind)
		canControl := tasks.Has(codes, c.Scope.Kind+".monitoring.jobs.manage")
		if c.Module == "derived" {
			canControl = tasks.Has(codes, c.Scope.Kind+".derived.runs.manage")
			owner, ok := s.executors[j.Kind].(TaskSummaryAuthorizer)
			if !ok {
				return nil, tasks.Problem("task_type_unavailable", "计算任务摘要尚未注册。", 503)
			}
			e = owner.AuthorizeTaskSummaryTx(ctx, tx, taskOf(j), a, mode == "managed")
			if taskHidden(e) {
				continue
			}
			if e != nil {
				return nil, e
			}
		}
		readable := false
		resultCondition := ""
		e = s.jobPermissionTx(ctx, tx, auth, j)
		if e == nil {
			readable = true
		} else if !taskHidden(e) {
			var me *Error
			if errors.As(e, &me) && slices.Contains([]string{"snapshot_expired", "export_expired", "trial_receipt_expired"}, me.Code) {
				resultCondition = "expired"
			} else if errors.As(e, &me) && slices.Contains([]string{"snapshot_stale", "export_invalidated", "artifact_invalidated"}, me.Code) {
				resultCondition = "invalidated"
			} else {
				return nil, e
			}
		}
		retained := resultCondition != "" && c.Originator == a.ID && tasks.CanUse(codes, d, c.Scope.Kind, "mine")
		if mode == "managed" && !manager || mode == "mine" && (c.Originator != a.ID || !readable && !manager && !retained) || mode == "detail" && !readable && !manager && !retained {
			continue
		}
		v := tasks.Present(c, label)
		v.ResultReadable = readable
		if c.Originator == a.ID {
			v.ActorLabel = "我"
		} else {
			v.ActorLabel = "其他成员"
		}
		if !readable {
			v.ResultState = "restricted"
			if resultCondition != "" {
				v.ResultState = resultCondition
			}
			v.ErrorCode = safeTaskReason(v.ErrorCode)
		}
		if c.Module == "monitoring" && readable {
			if j.Kind == "validate" || j.Kind == "publish" {
				canControl = canControl || tasks.Has(codes, c.Scope.Kind+".monitoring.imports.manage")
			} else {
				canControl = true
			}
		}
		tasks.Capability(&v, "cancel", canControl && taskrecord.Active(v.State) && v.State != "cancelling", c.Originator != a.ID, "task_not_active")
		if c.Module == "derived" {
			canRetry := canControl && slices.Contains([]string{"failed", "cancelled", "needs_action"}, v.State) && status != "archived"
			reason := "task_not_retryable"
			if canRetry {
				if e = s.executors[j.Kind].AuthorizeExecutionTx(ctx, tx, taskOf(j)); e != nil {
					if !taskHidden(e) {
						var domain interface{ StatusCode() int }
						var taskError *TaskError
						if !errors.As(e, &taskError) && (!errors.As(e, &domain) || domain.StatusCode() >= 500) {
							return nil, e
						}
					}
					canRetry = false
					reason = "execution_principal_unavailable"
				}
			}
			tasks.Capability(&v, "retry", canRetry, true, reason)
		}
		if readable {
			if e = s.taskResultStateTx(ctx, tx, j, &v); e != nil {
				return nil, e
			}
			if owner, ok := s.executors[j.Kind].(interface {
				TaskResultStateTx(context.Context, pgx.Tx, Task, access.Actor) (string, error)
			}); ok {
				state, err := owner.TaskResultStateTx(ctx, tx, taskOf(j), a)
				if err != nil {
					return nil, err
				}
				if state != "" {
					v.ResultState = state
				}
			}
		}
		if v.State == "succeeded" && (v.Type == "analysis.run" && !tasks.Has(codes, c.Scope.Kind+".derived.analysis.results.read") || v.Type == "derived.backfill" && !tasks.Has(codes, c.Scope.Kind+".derived.results.history")) {
			v.ResultState = "restricted"
		}
		out = append(out, v)
	}
	return out, nil
}
func safeTaskReason(code string) string {
	if code == "" {
		return ""
	}
	if slices.Contains([]string{"job_cancelled", "job_attempts_exhausted", "authorization_changed", "validation_requires_resolution", "snapshot_expired", "export_invalidated"}, code) {
		return code
	}
	return "task_execution_failed"
}
func (s *Service) taskResultStateTx(ctx context.Context, tx pgx.Tx, j Job, v *tasks.View) error {
	if j.Kind == "export" || j.Kind == "analysis_export" || j.Kind == "derived_evidence_export" {
		var expired, invalid bool
		e := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp(),invalidated_at IS NOT NULL FROM monitoring_exports WHERE job_id=$1`, j.ID).Scan(&expired, &invalid)
		if e == pgx.ErrNoRows {
			v.ResultState = "missing"
			return nil
		}
		if e != nil {
			return e
		}
		if invalid {
			v.ResultState = "invalidated"
		} else if expired {
			v.ResultState = "expired"
		}
		v.NextStep = "view_files"
	} else if j.Kind == "validate" || j.Kind == "publish" {
		var state string
		if e := tx.QueryRow(ctx, `SELECT state FROM monitoring_import_batches WHERE id=$1`, j.Target).Scan(&state); e != nil {
			if e == pgx.ErrNoRows {
				v.ResultState = "missing"
				return nil
			}
			return e
		}
		v.NextStep = "view_batch"
		if j.Kind == "validate" && state == "ready" {
			v.NextStep = "adopt"
		}
		if state == "revoked" {
			v.ResultState = "invalidated"
		}
		if state == "completed_noop" {
			v.NextStep = "completed_noop"
		}
	} else if j.Kind == "summary" {
		var expired bool
		if e := tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM monitoring_snapshots WHERE id=$1`, j.Target).Scan(&expired); e != nil {
			if e == pgx.ErrNoRows {
				v.ResultState = "missing"
				return nil
			}
			return e
		}
		if expired {
			v.ResultState = "expired"
		}
		v.NextStep = "view_query"
	} else {
		v.NextStep = "view_run"
		if v.Type == "derived.trial" && v.State == "succeeded" {
			var live bool
			if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_trial_receipts WHERE run_id=$1 AND expires_at>clock_timestamp())`, j.Target).Scan(&live); e != nil {
				return e
			}
			if !live {
				v.ResultState = "expired"
			}
		}
	}
	return nil
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, c tasks.Command) (tasks.CommandResult, error) {
	var out tasks.CommandResult
	if e := c.Validate(); e != nil {
		return out, e
	}
	if a.Check == nil || a.ReadSubject() != nil {
		return out, access.ErrDenied
	}
	e := s.db(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, false); e != nil {
			return e
		}
		views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		v := views[0]
		scope := Scope{Entry: v.Scope.Kind, Tenant: v.Scope.Tenant, Project: v.Scope.Project}
		auth, e := s.taskAuthTx(ctx, tx, a, scope)
		if e != nil {
			return e
		}
		// The original receipt namespace is retained. The complete source + command
		// is hashed under one operation so changing action cannot reuse the key.
		payload := []any{ref, c}
		hit, e := receiptTx(ctx, tx, auth, "task.control", c.RequestID, payload, &out)
		if e != nil {
			return e
		}
		if hit {
			out.Replayed = true
			return nil
		}
		if e = devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e = devicecoord.Tenant(ctx, tx, scope.Tenant, true); e != nil {
			return e
		}
		var job string
		e = tx.QueryRow(ctx, `SELECT id::text FROM monitoring_jobs WHERE tenant_id=$1 AND (id=$2 OR target_id=$2) AND kind=ANY($3::text[]) FOR UPDATE`, scope.Tenant, ref.RootID, append([]string{"validate", "publish", "export", "summary"}, calculationKinds...)).Scan(&job)
		if e != nil {
			return e
		}
		views, e = s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		v = views[0]
		if e = tasks.ValidateCommand(c, v, a); e != nil {
			return e
		}
		if e = tasks.SetCommandTx(ctx, tx, a, c, v); e != nil {
			return e
		}
		j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, job))
		if e != nil {
			return e
		}
		if c.Action == "retry" {
			executor := s.executors[j.Kind]
			if executor == nil {
				return tasks.Problem("task_action_denied", "请在原业务重新申请。", 403)
			}
			if e = executor.AuthorizeExecutionTx(ctx, tx, taskOf(j)); e != nil {
				return tasks.Problem("execution_principal_unavailable", "固定输入或原执行主体不可用，请在原页面重新申请。", 409)
			}
			_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='queued',notification_generation=notification_generation+1,attempts=0,fence=fence+1,cancel_requested=false,error_code=NULL,lease_owner=NULL,lease_until=NULL,available_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, job)
		} else {
			_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET cancel_requested=true,state=CASE WHEN state='running' THEN state ELSE 'cancelled' END,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, job)
			if e == nil && (j.Kind == "validate" || j.Kind == "publish") {
				_, e = tx.Exec(ctx, `UPDATE monitoring_import_batches SET state='cancelled',revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3 AND state IN ('validating','preparing')`, scope.Tenant, j.Target, j.TargetRevision)
			}
		}
		if e != nil {
			return e
		}
		if e = s.RecordTaskOutcomeTx(ctx, tx, job); e != nil {
			return e
		}
		views, e = s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		out = tasks.Outcome(views[0], false)
		if e = access.Audit(ctx, tx, "task."+c.Action, "task", v.ID, "", map[string]string{"state": v.State, "generation": v.Generation}, out, access.Meta{ActorID: a.ID, ActorType: "account", ActorScope: scope.Entry, TenantID: scope.Tenant, ProjectID: scope.Project, Reason: c.Reason, RequestID: c.RequestID}); e != nil {
			return e
		}
		return saveReceiptTx(ctx, tx, auth, "task.control", c.RequestID, payload, out)
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
	v := views[0]
	out := tasks.Target{Scope: v.Scope, Generation: generation, Reason: "result_restricted"}
	if !v.ResultReadable {
		return out, nil
	}
	if generation != v.Generation {
		out.Reason = "historical_result_unavailable"
		return out, nil
	}
	var c taskrecord.Change
	if e = tx.QueryRow(ctx, `SELECT snapshot FROM monitoring_task_metadata WHERE snapshot->>'source_module'=$1 AND snapshot->>'source_kind'=$2 AND snapshot->>'source_root_id'=$3`, ref.Module, ref.Kind, ref.RootID).Scan(&c); e != nil {
		return out, e
	}
	out.Available = true
	out.Reason = ""
	out.ID = c.InputRef["target_id"]
	out.Kind = "monitoring_job"
	out.Parameters = map[string]string{"job_id": c.InputRef["job_id"]}
	switch v.Type {
	case "monitoring.import.validate", "monitoring.import.adopt":
		out.Kind = "monitoring_import"
	case "monitoring.export":
		out.Kind = "monitoring_export"
	case "monitoring.query":
		out.Kind = "monitoring_query"
	case "analysis.export":
		out.Kind = "analysis_export"
	case "derived.evidence.export":
		out.Kind = "derived_evidence_export"
	default:
		out.Kind = "derived_run"
	}
	if kind != "result" {
		return out, nil
	}
	if v.ResultState != "available" {
		return tasks.Target{Scope: v.Scope, Generation: generation, Reason: "result_" + v.ResultState}, nil
	}
	if v.Type == "analysis.run" || v.Type == "derived.backfill" {
		executor := "analysis"
		if v.Type == "derived.backfill" {
			executor = "derive"
		}
		resolver, ok := s.executors[executor].(runResultResolver)
		if !ok {
			return tasks.Target{}, tasks.Problem("task_type_unavailable", "尚未注册计算成果读取服务。", 503)
		}
		out.Kind, out.ID, out.Parameters, e = resolver.TaskRunResultTx(ctx, tx, a, v.Scope, v.Type, out.ID)
		if taskHidden(e) {
			return tasks.Target{Scope: v.Scope, Generation: generation, Reason: "result_restricted"}, nil
		}
		if e != nil {
			return tasks.Target{}, e
		}
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter, module string) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, module, func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var q, r, failed string
		condition := `kind IN ('summary','verify','relocate','backup','restore')`
		if module == "derived" {
			counter, ok := s.executors["derive"].(automaticRunCounter)
			if !ok {
				return "", "", "", tasks.Problem("task_type_unavailable", "尚未注册自动计算运行服务。", 503)
			}
			return counter.TaskAutomaticCountsTx(ctx, tx, a, scope)
		}
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN ('queued','retry_wait'))::text,count(*) FILTER(WHERE state='running')::text,count(*) FILTER(WHERE state IN ('failed','needs_action'))::text FROM monitoring_jobs WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id IS NULL AND `+condition, scope.Tenant, taskrecord.Null(scope.Project)).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}

// The original source routes attach the same safe control context as /tasks.
func (s *Service) setTaskCommandTx(ctx context.Context, tx pgx.Tx, a access.Actor, id string, c tasks.Command) error {
	var fact *taskrecord.Change
	if e := tx.QueryRow(ctx, `SELECT snapshot FROM monitoring_task_metadata WHERE id=$1`, id).Scan(&fact); e != nil {
		return e
	}
	if fact == nil {
		return tasks.SetCommandTx(ctx, tx, a, c)
	}
	return tasks.SetCommandTx(ctx, tx, a, c, tasks.Present(*fact, ""))
}
