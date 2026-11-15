package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TaskExecutor extends the shared queue without importing a calculation domain
// into monitoring. Every execution transaction rechecks current qualification;
// an actor-less system job requires the executor's durable execution mandate.
type TaskExecutor interface {
	AuthorizeExecutionTx(context.Context, pgx.Tx, Task) error
	AuthorizeTaskTx(context.Context, pgx.Tx, Task, access.Actor, Scope, bool) error
	ExecuteTask(context.Context, TaskLease) error
}
type Task struct {
	Job     Job
	Scope   Scope
	ActorID string
	Fence   string
	Input   json.RawMessage
}

func taskOf(j Job) Task {
	return Task{j, j.scope(), j.actor, strconv.FormatInt(j.fence, 10), slices.Clone(j.input)}
}

var calculationKinds = []string{"derive", "analysis", "derived_replay", "analysis_compare", "analysis_export", "derived_evidence_export"}

// WithTaskExecutors is composition-time configuration, before serving requests
// or starting workers. The map is copied so callers cannot mutate registration.
func (s *Service) WithTaskExecutors(executors map[string]TaskExecutor) (*Service, error) {
	registered := map[string]TaskExecutor{}
	for kind, executor := range executors {
		if !slices.Contains(calculationKinds, kind) || executor == nil {
			return s, fmt.Errorf("unsupported calculation executor %q", kind)
		}
		registered[kind] = executor
	}
	s.executors = registered
	return s, nil
}
func (s *Service) claimKinds() []string {
	out := []string{"validate", "publish", "summary", "export", "relocate", "verify"}
	for kind := range s.executors {
		out = append(out, kind)
	}
	slices.Sort(out)
	return out
}

type EnqueueTask struct {
	ID                           string
	Scope                        Scope
	ActorID                      string
	Kind, Target, TargetRevision string
	Input                        json.RawMessage
	AvailableAt                  time.Time
}

// EnqueueTaskTx joins a domain command's already-authorized transaction. It does
// not open a nested transaction or commit a task before its run/config exists.
func (s *Service) EnqueueTaskTx(ctx context.Context, tx pgx.Tx, in EnqueueTask) (Job, error) {
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	if !access.ValidID(in.ID) {
		return Job{}, invalid("task", "无效任务标识。")
	}
	if s.executors[in.Kind] == nil || !in.Scope.valid() || !access.ValidID(in.Target) || in.ActorID != "" && !access.ValidID(in.ActorID) {
		return Job{}, invalid("task", "需要已注册执行器和有效的任务归属。")
	}
	revision, e := revisionNumber(in.TargetRevision)
	if e != nil {
		return Job{}, e
	}
	var object map[string]json.RawMessage
	if len(in.Input) > 256*1024 || json.Unmarshal(in.Input, &object) != nil || object == nil {
		return Job{}, invalid("input", "任务正文必须是有界 JSON 对象。")
	}
	var at any
	if !in.AvailableAt.IsZero() {
		at = in.AvailableAt
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO monitoring_jobs(tenant_id,project_id,actor_id,kind,target_id,target_revision,input_hash,input,available_at,id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9::timestamptz,clock_timestamp()),$10) ON CONFLICT(tenant_id,kind,target_id,target_revision,input_hash) DO UPDATE SET input_hash=EXCLUDED.input_hash RETURNING id::text`, in.Scope.Tenant, nullable(in.Scope.Project), nullable(in.ActorID), in.Kind, in.Target, revision, digest(object), in.Input, at, in.ID).Scan(&id)
	if e != nil {
		return Job{}, e
	}
	return scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2`, in.Scope.Tenant, id))
}
func (s *Service) executionTransaction(ctx context.Context, j Job, executor TaskExecutor, fn func(pgx.Tx) error) error {
	return s.db(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if j.actor != "" {
			if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, j.actor); e != nil {
				return e
			}
			if e := workerActor(j.actor).Check(ctx, tx, false); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e := devicecoord.Catalog(ctx, tx, false); e != nil {
			return e
		}
		if e := devicecoord.Tenant(ctx, tx, j.tenant, false); e != nil {
			return e
		}
		var ready bool
		if e := tx.QueryRow(ctx, `SELECT mode='ready' FROM monitoring_environment_state WHERE id=1`).Scan(&ready); e != nil {
			return e
		}
		if !ready {
			return problem("monitoring_restoring", "监测环境尚未恢复就绪。", 503)
		}
		if e := executor.AuthorizeExecutionTx(ctx, tx, taskOf(j)); e != nil {
			return e
		}
		return fn(tx)
	})
}

// TaskLease cannot be constructed by callers. All mutation methods check both
// the shared lease fence and domain qualification under the same transaction.
type TaskLease struct {
	service *Service
	job     Job
}

func (l TaskLease) Task() Task          { return taskOf(l.job) }
func (l TaskLease) Actor() access.Actor { return workerActor(l.job.actor) }
func (l TaskLease) Transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	if l.service == nil {
		return access.ErrDenied
	}
	return l.service.jobTransaction(ctx, l.job, func(tx pgx.Tx) error {
		if e := assertJobTx(ctx, tx, l.job); e != nil {
			return e
		}
		return fn(tx)
	})
}
func (l TaskLease) Check(ctx context.Context) error {
	return l.Transaction(ctx, func(pgx.Tx) error { return nil })
}
func (l TaskLease) Progress(ctx context.Context, progress map[string]string) error {
	if l.service == nil {
		return access.ErrDenied
	}
	return l.service.recordProgress(ctx, l.job, progress)
}

type TaskCheckpoint struct {
	State  string
	Fence  string
	Result json.RawMessage
}

func (l TaskLease) Checkpoint(ctx context.Context, key string) (TaskCheckpoint, bool, error) {
	var out TaskCheckpoint
	found := false
	e := l.Transaction(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT state,fence::text,result FROM monitoring_job_steps WHERE tenant_id=$1 AND job_id=$2 AND step_key=$3`, l.job.tenant, l.job.ID, key).Scan(&out.State, &out.Fence, &out.Result)
		if e == pgx.ErrNoRows {
			return nil
		}
		found = e == nil
		return e
	})
	return out, found, e
}
func (l TaskLease) SaveCheckpoint(ctx context.Context, key, state string, result any) error {
	raw, e := json.Marshal(result)
	if e != nil {
		return e
	}
	if len(key) == 0 || len(key) > 200 || len(raw) > 256*1024 || !slices.Contains([]string{"running", "completed", "failed"}, state) {
		return invalid("checkpoint", "检查点必须有界且状态有效。")
	}
	return l.Transaction(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_job_steps(tenant_id,job_id,step_key,fence,state,result) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,job_id,step_key) DO UPDATE SET fence=EXCLUDED.fence,state=EXCLUDED.state,result=EXCLUDED.result,updated_at=clock_timestamp() WHERE monitoring_job_steps.fence<=EXCLUDED.fence`, l.job.tenant, l.job.ID, key, l.job.fence, state, raw)
		return e
	})
}

// TaskError classifies domain failures without coupling the host to a rule
// compiler. Permanent budget/engine/configuration failures need human action.
// The background disposition returns an interactive attempt to the queue
// immediately without charging its failure budget.
type TaskError struct{ Code, Message, Disposition string }

func (e *TaskError) Error() string { return e.Message }

type interactiveTaskKey struct{}

// RunInteractiveTask applies a wall-clock budget to the entire claimed task,
// including authorization and readiness checks. Some policy adapters hide
// database failures behind a safe access error, so the host observes its own
// deadline instead of relying on every adapter to preserve that error chain.
// A background worker must still repeat all authorization checks.
func (s *Service) RunInteractiveTask(ctx context.Context, owner, tenant, id string, budget time.Duration) (bool, error) {
	short, cancel := context.WithTimeout(context.WithValue(ctx, interactiveTaskKey{}, true), budget)
	defer cancel()
	return s.RunTask(short, owner, tenant, id)
}

// TaskTx and ControlTaskTx are seams for a domain's run views and receipts.
// They always invoke the registered executor's current read/manage policy.
func (s *Service) TaskTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, manage bool) (Task, error) {
	j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id))
	if e != nil {
		return Task{}, e
	}
	executor := s.executors[j.Kind]
	if executor == nil {
		return Task{}, notFound()
	}
	if e = executor.AuthorizeTaskTx(ctx, tx, taskOf(j), a, scope, manage); e != nil {
		return Task{}, e
	}
	return taskOf(j), nil
}

// TaskCommandResultTx is limited to the requesting actor's own command result.
// Browsing and managing tasks use the executor's independent runs permissions.
func (s *Service) TaskCommandResultTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (Task, error) {
	j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2 AND actor_id=$3 AND project_id IS NOT DISTINCT FROM $4::uuid`, scope.Tenant, id, a.ID, nullable(scope.Project)))
	if e != nil {
		return Task{}, e
	}
	executor := s.executors[j.Kind]
	if executor == nil {
		return Task{}, notFound()
	}
	if e = executor.AuthorizeExecutionTx(ctx, tx, taskOf(j)); e != nil {
		return Task{}, e
	}
	return taskOf(j), nil
}
func (s *Service) ControlTaskTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id, expected, action string, taskCommands ...tasks.Command) (Job, error) {
	task, e := s.TaskTx(ctx, tx, a, scope, id, true)
	if e != nil {
		return Job{}, e
	}
	j := task.Job
	if j.Revision != expected {
		return Job{}, revisionChanged()
	}
	var command string
	switch action {
	case "cancel":
		if !slices.Contains([]string{"queued", "retry_wait", "running"}, j.State) {
			return Job{}, revisionChanged()
		}
		command = `UPDATE monitoring_jobs SET cancel_requested=true,state=CASE WHEN state='running' THEN state ELSE 'cancelled' END,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3`
	case "retry":
		if !slices.Contains([]string{"failed", "needs_action", "cancelled"}, j.State) {
			return Job{}, revisionChanged()
		}
		command = `UPDATE monitoring_jobs SET state='queued',notification_generation=notification_generation+1,attempts=0,fence=fence+1,cancel_requested=false,error_code=NULL,lease_owner=NULL,lease_until=NULL,available_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND revision=$3`
	default:
		return Job{}, invalid("action", "需要取消或重试操作。")
	}
	contextCommand := tasks.Command{Action: action}
	if len(taskCommands) > 0 {
		contextCommand = taskCommands[0]
		contextCommand.Action = action
	}
	if e = s.setTaskCommandTx(ctx, tx, a, j.ID, contextCommand); e != nil {
		return Job{}, e
	}
	tag, e := tx.Exec(ctx, command, scope.Tenant, id, expected)
	if e != nil {
		return Job{}, e
	}
	if tag.RowsAffected() != 1 {
		return Job{}, revisionChanged()
	}
	if e = s.RecordTaskOutcomeTx(ctx, tx, id); e != nil {
		return Job{}, e
	}
	return scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id))
}
