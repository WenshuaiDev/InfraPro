package derivedmonitoring_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// This exercises the host's database protocol, not physical result storage.
type hostExecutor struct {
	allowed atomic.Bool
	run     func(context.Context, monitoring.TaskLease) error
}

func TestCalculationReservedQueuesAndTenantFairness(t *testing.T) {
	f := setup(t)
	a := f.actor(f.admin)
	second, e := tenancy.New(f.e.Service.Access()).Create(ctx, f.actor(f.platform), tenancy.CreateInput{Fields: tenancy.Fields{Name: "第二租户", Code: "derived-fairness"}, AdministratorID: a.ID, RequestID: uuid.NewString(), Reason: "验证公平领取"}, access.Meta{})
	must(t, e)
	x := &hostExecutor{}
	x.allowed.Store(true)
	host, e := monitoring.New(f.e.Service.Access()).WithStorage(new(monitoring.Storage)).WithTaskExecutors(map[string]monitoring.TaskExecutor{"derive": x, "analysis": x})
	must(t, e)
	enqueue := func(tenant, kind, input string) monitoring.Job {
		t.Helper()
		var j monitoring.Job
		must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			var e error
			j, e = host.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{Scope: monitoring.TenantScope(tenant), ActorID: a.ID, Kind: kind, Target: uuid.NewString(), TargetRevision: "1", Input: json.RawMessage(input)})
			return e
		}))
		return j
	}
	var claimed []string
	x.run = func(_ context.Context, l monitoring.TaskLease) error {
		claimed = append(claimed, l.Task().Scope.Tenant)
		return nil
	}
	for range 4 {
		enqueue(f.scope.Tenant, "derive", `{"mode":"trial"}`)
	}
	enqueue(second.TenantID, "derive", `{"mode":"trial"}`)
	for range 5 {
		worked, e := host.RunOne(ctx, "fairness")
		must(t, e)
		if !worked {
			t.Fatal("queued work missing")
		}
	}
	if len(claimed) != 5 || claimed[0] == claimed[1] {
		t.Fatal("one tenant occupied both initial claims", claimed)
	}
	started, interactive := make(chan struct{}), make(chan string, 2)
	blocked := enqueue(f.scope.Tenant, "derive", `{"mode":"backfill"}`)
	x.run = func(c context.Context, l monitoring.TaskLease) error {
		if l.Task().Job.ID == blocked.ID {
			close(started)
			<-c.Done()
			return c.Err()
		}
		interactive <- l.Task().Job.Kind
		return nil
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- host.RunWorker(workerCtx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	t.Cleanup(func() { stop(); <-done })
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("reserved historical lane did not run")
	}
	enqueue(second.TenantID, "derive", `{"mode":"automatic"}`)
	enqueue(second.TenantID, "analysis", `{"mode":"analysis","trial":true}`)
	for range 2 {
		select {
		case <-interactive:
		case <-time.After(5 * time.Second):
			t.Fatal("historical job blocked automatic calculation or analysis trial")
		}
	}
	start := time.Now()
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		s := monitoring.TenantScope(f.scope.Tenant)
		task, e := host.TaskTx(ctx, tx, a, s, blocked.ID, true)
		if e != nil {
			return e
		}
		_, e = host.ControlTaskTx(ctx, tx, a, s, blocked.ID, task.Job.Revision, "cancel")
		return e
	}))
	for {
		var state string
		must(t, f.e.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, blocked.ID).Scan(&state))
		if state == "cancelled" {
			break
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("reserved lane ignored cancellation")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (x *hostExecutor) AuthorizeExecutionTx(_ context.Context, _ pgx.Tx, t monitoring.Task) error {
	if !x.allowed.Load() || t.ActorID == "" {
		return access.ErrDenied
	}
	return nil
}
func (x *hostExecutor) AuthorizeTaskTx(_ context.Context, _ pgx.Tx, t monitoring.Task, a access.Actor, s monitoring.Scope, _ bool) error {
	if t.ActorID != a.ID || t.Scope != s {
		return access.ErrDenied
	}
	return nil
}
func (x *hostExecutor) ExecuteTask(ctx context.Context, l monitoring.TaskLease) error {
	return x.run(ctx, l)
}

func TestCalculationSharedTaskLeaseFenceAndCancellation(t *testing.T) {
	f := setup(t)
	actor := f.actor(f.admin)
	scope := monitoring.TenantScope(f.scope.Tenant)
	executor := &hostExecutor{}
	executor.allowed.Store(true)
	host, e := monitoring.New(f.e.Service.Access()).WithStorage(new(monitoring.Storage)).WithTaskExecutors(map[string]monitoring.TaskExecutor{"derive": executor})
	must(t, e)
	enqueue := func() monitoring.Job {
		t.Helper()
		var j monitoring.Job
		must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			var e error
			j, e = host.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{Scope: scope, ActorID: actor.ID, Kind: "derive", Target: uuid.NewString(), TargetRevision: "1", Input: json.RawMessage(`{"mode":"trial"}`)})
			return e
		}))
		return j
	}
	oldStarted := make(chan monitoring.TaskLease, 1)
	release := make(chan struct{})
	finished := make(chan error, 1)
	var calls atomic.Int32
	executor.run = func(c context.Context, l monitoring.TaskLease) error {
		if calls.Add(1) == 1 {
			oldStarted <- l
			<-release
			return l.SaveCheckpoint(context.Background(), "sealed", "completed", map[string]string{"attempt": "old"})
		}
		if e := l.SaveCheckpoint(c, "sealed", "completed", map[string]string{"attempt": "new"}); e != nil {
			return e
		}
		return l.Progress(c, map[string]string{"phase": "publishing", "rows": "3"})
	}
	job := enqueue()
	go func() { _, e := host.RunOne(ctx, "old-worker"); finished <- e }()
	var old monitoring.TaskLease
	select {
	case old = <-oldStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("host did not claim registered task")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE monitoring_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, job.ID)
	must(t, e)
	worked, e := host.RunOne(ctx, "new-worker")
	must(t, e)
	if !worked {
		t.Fatal("expired task was not reclaimed")
	}
	e = old.Transaction(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE monitoring_jobs SET progress='{"overwritten":true}' WHERE id=$1`, job.ID)
		return e
	})
	monitoringCode(t, e, "job_lease_lost")
	close(release)
	select {
	case e = <-finished:
		monitoringCode(t, e, "job_lease_lost")
	case <-time.After(5 * time.Second):
		t.Fatal("old worker did not exit")
	}
	var state, fence, attempt string
	must(t, f.e.Pool.QueryRow(ctx, `SELECT j.state,s.fence::text,s.result->>'attempt' FROM monitoring_jobs j JOIN monitoring_job_steps s ON s.job_id=j.id WHERE j.id=$1`, job.ID).Scan(&state, &fence, &attempt))
	if state != "succeeded" || fence != "2" || attempt != "new" {
		t.Fatal(state, fence, attempt)
	}
	executor.run = func(context.Context, monitoring.TaskLease) error {
		return &monitoring.TaskError{Code: "numeric_budget_exceeded", Message: "bounded arithmetic budget", Disposition: "needs_action"}
	}
	budgetJob := enqueue()
	_, e = host.RunOne(ctx, "budget-worker")
	if e == nil {
		t.Fatal("budget failure hidden")
	}
	must(t, f.e.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, budgetJob.ID).Scan(&state))
	if state != "needs_action" {
		t.Fatal("permanent budget failure retried", state)
	}
	// Waiting for an authorized upstream publication is neither success nor a
	// failed retry attempt. It remains cancellable through the ordinary host.
	executor.run = func(context.Context, monitoring.TaskLease) error {
		return &monitoring.TaskError{Code: "upstream_pending", Message: "awaiting upstream", Disposition: "waiting"}
	}
	waiting := enqueue()
	for attempt := 0; attempt < 6; attempt++ {
		_, e = host.RunTask(ctx, "waiting-worker", scope.Tenant, waiting.ID)
		if e == nil {
			t.Fatal("waiting task reported success")
		}
		var tries int
		must(t, f.e.Pool.QueryRow(ctx, `SELECT state,attempts FROM monitoring_jobs WHERE id=$1`, waiting.ID).Scan(&state, &tries))
		if state != "retry_wait" || tries != 0 {
			t.Fatal("dependency wait exhausted failure budget", state, tries)
		}
		_, e = f.e.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE id=$1`, waiting.ID)
		must(t, e)
	}
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		task, e := host.TaskTx(ctx, tx, actor, scope, waiting.ID, true)
		if e != nil {
			return e
		}
		_, e = host.ControlTaskTx(ctx, tx, actor, scope, waiting.ID, task.Job.Revision, "cancel")
		return e
	}))
	started := make(chan struct{})
	executor.run = func(c context.Context, _ monitoring.TaskLease) error { close(started); <-c.Done(); return c.Err() }
	cancelled := enqueue()
	go func() { _, e := host.RunOne(ctx, "cancel-worker"); finished <- e }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("cancel task did not start")
	}
	at := time.Now()
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		task, e := host.TaskTx(ctx, tx, actor, scope, cancelled.ID, true)
		if e != nil {
			return e
		}
		_, e = host.ControlTaskTx(ctx, tx, actor, scope, cancelled.ID, task.Job.Revision, "cancel")
		return e
	}))
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation exceeded two-second propagation target")
	}
	if time.Since(at) > 2*time.Second {
		t.Fatal("cancellation exceeded target")
	}
	must(t, f.e.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, cancelled.ID).Scan(&state))
	if state != "cancelled" {
		t.Fatal(state)
	}
	// The persistent job owner is not a permanent authority token.
	executor.allowed.Store(false)
	denied := enqueue()
	_, e = host.RunOne(ctx, "revoked-worker")
	if e == nil {
		t.Fatal("revoked task still executed")
	}
	must(t, f.e.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, denied.ID).Scan(&state))
	if state != "needs_action" {
		t.Fatal(state)
	}
}
