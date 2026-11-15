package resources_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTaskRecordAtomicHistoryAndOutOfOrder(t *testing.T) {
	f := resourcesSetup(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	source := taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: uuid.NewString()}
	c := taskrecord.Change{SourceRef: source, Type: "monitoring.export", Scope: taskrecord.Scope{Kind: "tenant", Tenant: f.tenant}, Originator: f.actor.ID, OriginKind: "person", RequestedBy: f.actor.ID, ExecutorKind: "person", ExecutorRef: f.actor.ID, Generation: "1", Revision: "1", ControlRevision: "1", State: "queued", SourceState: "queued", Phase: "queued", InputRef: map[string]string{"job_id": source.RootID}, InputHash: "fixed", ResultRef: map[string]string{}, CreatedAt: now, ObservedAt: now, Fence: "0", Event: "queued"}
	apply := func(c taskrecord.Change) error {
		return f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := taskrecord.ApplyTx(ctx, tx, c); return e })
	}
	must(t, apply(c))
	must(t, apply(c))
	changed := c
	changed.InputHash = "tampered"
	if !errors.Is(apply(changed), taskrecord.ErrConflict) {
		t.Fatal("same revision accepted different fact")
	}
	running := c
	running.State = "running"
	running.Phase = "writing"
	running.Revision = "2"
	running.Fence = "1"
	running.Attempt = 1
	running.ObservedAt = now.Add(time.Second)
	failed := running
	failed.State = "failed"
	failed.Phase = "failed"
	failed.Revision = "3"
	failed.ErrorCode = "synthetic_failure"
	failed.ObservedAt = now.Add(2 * time.Second)
	// Terminal arrives first; the genuine old claim must remain a closed old attempt.
	must(t, apply(failed))
	must(t, apply(running))
	must(t, apply(failed))
	retry := failed
	retry.State = "queued"
	retry.Phase = "queued"
	retry.Generation = "2"
	retry.Revision = "4"
	retry.ErrorCode = ""
	retry.ObservedAt = now.Add(3 * time.Second)
	retry.Reason = "固定输入人工重试"
	must(t, apply(retry))
	must(t, apply(running))
	var generation, state, reason, outcome string
	var closed bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_generation::text,state FROM task_records WHERE id=$1`, source.ID()).Scan(&generation, &state))
	if generation != "2" || state != "queued" {
		t.Fatal("late fact replaced current execution")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT reason_code FROM task_executions WHERE task_id=$1 AND generation=1`, source.ID()).Scan(&reason))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT outcome,finished_at IS NOT NULL FROM task_attempts WHERE task_id=$1 AND generation=1`, source.ID()).Scan(&outcome, &closed))
	if reason != "synthetic_failure" || outcome != "failed" || !closed {
		t.Fatalf("history lost %s %s %v", reason, outcome, closed)
	}
	e := f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		c := retry
		c.Revision = "5"
		c.State = "running"
		c.Phase = "computing"
		if _, e := taskrecord.ApplyTx(ctx, tx, c); e != nil {
			return e
		}
		return fmt.Errorf("rollback")
	})
	if e == nil {
		t.Fatal("rollback not requested")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM task_records WHERE id=$1`, source.ID()).Scan(&state))
	if state != "queued" {
		t.Fatal("fact escaped rollback")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE task_executions SET reason_code='rewritten' WHERE task_id=$1 AND generation=1`, source.ID())
	if e == nil {
		t.Fatal("closed execution mutable")
	}
}

type taskDisplay struct {
	f            resourceFixture
	p            projectFixture
	b            runtime.Business
	s            *display.Service
	id, revision string
	scope        display.Scope
}

func newTaskDisplay(t *testing.T, f resourceFixture, name string) *taskDisplay {
	p := f.project(t, name)
	b := runtime.Compose(f.env.Service.Access())
	s := b.Displays.WithStorage(&displayMemoryStore{files: map[string][]byte{}})
	h := &taskDisplay{f: f, p: p, b: b, s: s, scope: display.Scope{Tenant: f.tenant, Project: p.id}, revision: "0"}
	_, e := s.Command(ctx, p.actor, h.scope, "", "settings", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "任务中心合成测试"}, Enabled: true})
	must(t, e)
	r, e := s.Command(ctx, p.actor, h.scope, "", "create", "", display.Command{Meta: display.Meta{Revision: "0", RequestID: uuid.NewString(), Reason: "任务中心合成测试"}, Name: name})
	must(t, e)
	h.id = r["id"].(string)
	h.revision = r["revision"].(string)
	return h
}
func (h *taskDisplay) command(t *testing.T, action, target string, c display.Command) map[string]any {
	t.Helper()
	c.Meta = display.Meta{Revision: h.revision, RequestID: uuid.NewString(), Reason: "任务中心合成测试"}
	r, e := h.s.Command(ctx, h.p.actor, h.scope, h.id, action, target, c)
	must(t, e)
	if v, ok := r["revision"].(string); ok {
		h.revision = v
	}
	return r
}
func (h *taskDisplay) upload(t *testing.T, data []byte) taskrecord.SourceRef {
	t.Helper()
	u := h.command(t, "upload", "", display.Command{Size: int64(len(data)), SHA256: displayHash(data)})["upload_id"].(string)
	must(t, h.s.UploadPart(ctx, h.p.actor, h.scope, h.id, u, 0, displayHash(data), data))
	h.command(t, "upload.complete", u, display.Command{})
	var id string
	must(t, h.f.env.Pool.QueryRow(ctx, `SELECT id::text FROM display_package_jobs WHERE upload_id=$1`, u).Scan(&id))
	return taskrecord.SourceRef{Module: "display", Kind: "job", RootID: id}
}
func TestTaskCenterDisplayRealLifecycleAndIndexIndependence(t *testing.T) {
	f := resourcesSetup(t)
	h := newTaskDisplay(t, f, "TaskDisplay")
	source := h.upload(t, []byte("not a ZIP"))
	s := runtime.ComposeTasks(f.env.Service.Access(), h.b)
	index := runtime.ComposeTaskIndexer(f.env.Service.Access())
	_, e := s.Resolve(ctx, h.p.actor, source)
	if e == nil {
		t.Fatal("unconsumed handoff claimed indexed")
	}
	_, e = index.Tick(ctx, "task-test")
	must(t, e)
	first, e := s.Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	if first.State != "queued" {
		t.Fatal(first.State)
	}
	worked, e := h.s.ProcessOne(ctx)
	if e == nil {
		t.Fatal("malformed package should fail validation")
	}
	if !worked {
		t.Fatal("worker skipped real source")
	}
	// Reads use live metadata even while indexer/notification delivery is stopped.
	failed, e := s.Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	if failed.State != "failed" {
		t.Fatal(failed.State)
	}
	_, e = index.Tick(ctx, "task-test")
	must(t, e)
	cmd := tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: failed.Generation, ExpectedRevision: failed.ControlRevision, Reason: "固定上传内容重试"}
	out, e := s.Command(ctx, h.p.actor, source.ID(), cmd)
	must(t, e)
	if out.Generation != "2" {
		t.Fatalf("retry generation %s", out.Generation)
	}
	replay, e := s.Command(ctx, h.p.actor, source.ID(), cmd)
	must(t, e)
	if replay.Generation != out.Generation {
		t.Fatal("receipt did not replay")
	}
	mismatch := cmd
	mismatch.Reason = "不同请求正文"
	if _, e = s.Command(ctx, h.p.actor, source.ID(), mismatch); e == nil {
		t.Fatal("request reuse accepted")
	}
	_, e = index.Tick(ctx, "task-test")
	must(t, e)
	old, e := s.Get(ctx, h.p.actor, source.ID(), "1")
	must(t, e)
	if old.State != "failed" || old.SelectedExecution.ReasonCode == "" || len(old.Actions) != 0 || len(old.Attempts) != 1 {
		t.Fatalf("old execution lost: %+v", old)
	}
	current, e := s.Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	_, e = s.Command(ctx, h.p.actor, source.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: current.Generation, ExpectedRevision: current.ControlRevision, Reason: "取消重新排队的同一上传"})
	must(t, e)
	_, e = index.Tick(ctx, "task-test")
	must(t, e)
	must(t, index.Verify(ctx))
	stopped, e := s.Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	if stopped.State != "cancelled" {
		t.Fatal(stopped.State)
	}
	page, e := s.List(ctx, h.p.actor, tasks.Filter{Limit: 1})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ID != source.ID() {
		t.Fatalf("public list %+v", page)
	}
	// A platform administrator does not gain project summaries automatically.
	hidden, e := s.List(ctx, f.fixture.actor(f.p), tasks.Filter{Mode: "managed"})
	must(t, e)
	if len(hidden.Items) != 0 {
		t.Fatal("platform crossed project boundary")
	}
	_, e = s.Get(context.Background(), f.fixture.actor(f.p), source.ID(), "")
	if !tasks.Denied(e) {
		t.Fatalf("hidden detail: %v", e)
	}
}

// This assertion is attached to the existing real source fixtures by the task
// suite. It checks their outputs through the public service with storage absent.
func verifyTaskSources(t *testing.T, f resourceFixture) {
	t.Helper()
	index := runtime.ComposeTaskIndexer(f.env.Service.Access())
	for n := 0; n < 20; n++ {
		_, e := index.Tick(ctx, "source-contract")
		must(t, e)
		var remaining int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM task_transaction_facts)+(SELECT count(*) FROM display_task_handoffs WHERE translated_at IS NULL AND attempts<8)`).Scan(&remaining))
		if remaining == 0 {
			break
		}
	}
	must(t, index.Backfill(ctx, "all"))
	must(t, index.Verify(ctx))
	rows, e := f.env.Pool.Query(ctx, `SELECT snapshot FROM task_records ORDER BY created_at,id`)
	must(t, e)
	facts := []taskrecord.Change{}
	for rows.Next() {
		var c taskrecord.Change
		must(t, rows.Scan(&c))
		facts = append(facts, c)
	}
	must(t, rows.Err())
	rows.Close()
	service := runtime.ComposeTasks(f.env.Service.Access())
	for _, c := range facts {
		must(t, c.Validate())
		if c.Originator == "" {
			continue
		}
		actor := f.actor
		actor.ID = c.Originator
		actor.Check = func(ctx context.Context, tx pgx.Tx, _ bool) error {
			var active bool
			e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, c.Originator).Scan(&active)
			if e != nil {
				return e
			}
			if !active {
				return tasks.Hidden()
			}
			return nil
		}
		detail, e := service.Get(ctx, actor, c.SourceRef.ID(), "")
		if tasks.Denied(e) {
			continue
		} // tests intentionally revoke a source principal
		must(t, e)
		if detail.Type != c.Type || detail.Generation != c.Generation || detail.State != c.State && c.Type != "derived.backfill_group" {
			t.Fatalf("source metadata mismatch %s: %+v", c.Type, detail)
		}
		if !c.HistoryIncomplete && taskrecord.Terminal(c.State) && detail.SelectedExecution.FinishedAt == nil {
			t.Fatalf("unclosed execution %s", c.Type)
		}
		t.Logf("real task source verified: %s (%s)", c.Type, c.State)
	}
}

func TestTaskIndexRestrictedRole(t *testing.T) {
	f := resourcesSetup(t)
	h := newTaskDisplay(t, f, "IndexRole")
	source := h.upload(t, displayZIP(t))
	_, e := h.s.ProcessOne(ctx)
	must(t, e)
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	_, e = f.env.Pool.Exec(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO infrapro_task_index")
	must(t, e)
	pc, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	pc.ConnConfig.RuntimeParams["role"] = "infrapro_task_index"
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	must(t, e)
	defer pool.Close()
	for _, table := range []string{"accounts", "notification_events", "monitoring_jobs", "spatial_processing_jobs", "display_package_jobs", "monitoring_task_execution_evidence", "spatial_task_execution_evidence", "display_task_execution_evidence"} {
		if _, e = pool.Exec(ctx, "SELECT * FROM "+table+" LIMIT 1"); e == nil {
			t.Fatalf("index role read forbidden %s", table)
		}
	}
	for _, table := range []string{"monitoring_jobs", "spatial_processing_jobs", "display_package_jobs"} {
		if _, e = pool.Exec(ctx, "UPDATE "+table+" SET state=state WHERE false"); e == nil {
			t.Fatalf("index role mutated %s", table)
		}
	}
	index := runtime.ComposeTaskIndexer(access.New(pool, slog.Default()))
	_, e = index.Tick(ctx, "least-privilege")
	must(t, e)
	must(t, index.Verify(ctx))
	detail, e := runtime.ComposeTasks(f.env.Service.Access(), h.b).Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	if detail.State != "succeeded" {
		t.Fatal(detail.State)
	}
	// Tampered handoff is diagnosed without losing an independently valid source.
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_task_handoffs SET translated_at=NULL,fingerprint='invalid' WHERE job_id=$1 AND task_revision=1`, source.RootID)
	must(t, e)
	_, e = index.Tick(ctx, "least-privilege")
	must(t, e)
	var code string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT last_error_code FROM display_task_handoffs WHERE job_id=$1 AND task_revision=1`, source.RootID).Scan(&code))
	if !strings.Contains(code, "failed") {
		t.Fatal("invalid handoff was silently accepted")
	}
}

func (s *taskVisibilitySource) ExistsTx(context.Context, pgx.Tx, taskrecord.SourceRef) (bool, error) {
	return true, nil
}

type taskVisibilitySource struct {
	values map[string]tasks.View
	broken bool
}

func (s *taskVisibilitySource) ResolveTx(ctx context.Context, tx pgx.Tx, _ access.Actor, refs []taskrecord.SourceRef, _ string) ([]tasks.View, error) {
	if s.broken {
		_, e := tx.Exec(ctx, `SELECT * FROM intentionally_missing_task_source`)
		return nil, e
	}
	out := []tasks.View{}
	for _, r := range refs {
		if v, ok := s.values[r.ID()]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *taskVisibilitySource) Control(context.Context, access.Actor, taskrecord.SourceRef, tasks.Command) (tasks.CommandResult, error) {
	return tasks.CommandResult{}, access.ErrDenied
}
func (s *taskVisibilitySource) TargetTx(context.Context, pgx.Tx, access.Actor, taskrecord.SourceRef, string, string) (tasks.Target, error) {
	return tasks.Target{}, access.ErrDenied
}
func (s *taskVisibilitySource) OperationsTx(context.Context, pgx.Tx, access.Actor, tasks.Filter) ([]tasks.Operation, error) {
	return nil, nil
}
func TestTaskPostAuthorizationPageBudgetAndStaleCursor(t *testing.T) {
	f := resourcesSetup(t)
	source := &taskVisibilitySource{values: map[string]tasks.View{}}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var recent taskrecord.Change
	for n := 0; n < 101; n++ {
		ref := taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: uuid.NewString()}
		c := taskrecord.Change{SourceRef: ref, Type: "monitoring.export", Scope: taskrecord.Scope{Kind: "tenant", Tenant: f.tenant}, Originator: f.actor.ID, OriginKind: "person", RequestedBy: f.actor.ID, ExecutorKind: "person", ExecutorRef: f.actor.ID, Generation: "1", Revision: "1", ControlRevision: "1", State: "queued", SourceState: "queued", Phase: "queued", InputRef: map[string]string{"job_id": ref.RootID}, InputHash: "fixed", CreatedAt: now.Add(-time.Duration(n) * time.Minute), ObservedAt: now, Fence: "0", Event: "queued"}
		must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := taskrecord.ApplyTx(ctx, tx, c); return e }))
		if n%30 == 0 {
			source.values[ref.ID()] = tasks.Present(c, "合成任务查询契约")
		}
		recent = c
	}
	// All candidates are real immutable metadata facts; this finite adapter models
	// per-object authorization to exercise the query algorithm, not source coverage.
	o := tasks.DefaultOptions()
	o.CandidateBatch = 2
	o.CandidateBudget = 101
	s := tasks.New(f.env.Service.Access(), map[string]tasks.Source{"monitoring": source}).WithOptions(o)
	first, e := s.List(ctx, f.actor, tasks.Filter{Limit: 2})
	must(t, e)
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatal("pre-auth limit created false end", first)
	}
	next, e := s.List(ctx, f.actor, tasks.Filter{Limit: 2, Cursor: first.NextCursor})
	must(t, e)
	if len(next.Items) != 2 || next.NextCursor != "" {
		t.Fatal("last authorized page incorrect", next)
	}
	for _, v := range next.Items {
		for _, prior := range first.Items {
			if v.ID == prior.ID {
				t.Fatal("repeated visible item")
			}
		}
	}
	if _, e = s.List(ctx, f.actor, tasks.Filter{Limit: 2, Cursor: first.NextCursor + "x"}); e == nil {
		t.Fatal("forged cursor accepted")
	}
	// The exact candidate limit is a true end when no candidates remain.
	all, e := s.List(ctx, f.actor, tasks.Filter{Limit: 10})
	must(t, e)
	if len(all.Items) != 4 {
		t.Fatal("candidate-budget boundary lost result")
	}
	// A new submission between page requests appears only after refreshing the
	// first page, without disturbing the old cursor's fixed upper boundary.
	recent.SourceRef.RootID = uuid.NewString()
	recent.InputRef = map[string]string{"job_id": recent.SourceRef.RootID}
	recent.CreatedAt = time.Now().UTC()
	recent.ObservedAt = recent.CreatedAt
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := taskrecord.ApplyTx(ctx, tx, recent); return e }))
	source.values[recent.SourceRef.ID()] = tasks.Present(recent, "翻页期间新提交")
	stable, e := s.List(ctx, f.actor, tasks.Filter{Limit: 2, Cursor: first.NextCursor})
	must(t, e)
	if len(stable.Items) != 2 || stable.NextCursor != "" || stable.Items[0].ID != next.Items[0].ID || stable.Items[1].ID != next.Items[1].ID {
		t.Fatal("new submission disturbed cursor page", stable)
	}
	home, e := s.List(ctx, f.actor, tasks.Filter{Limit: 2})
	must(t, e)
	if len(home.Items) != 2 || home.Items[0].ID != recent.SourceRef.ID() {
		t.Fatal("refreshed first page omitted new submission", home)
	}
	o.CandidateBudget = 3
	s.WithOptions(o)
	if _, e = s.List(ctx, f.actor, tasks.Filter{Limit: 2}); e == nil {
		t.Fatal("budget exhausted returned fake complete page")
	}
	sum, e := s.Summary(ctx, f.actor, tasks.Filter{})
	must(t, e)
	if sum.CountsState != "unavailable" || sum.Total != nil {
		t.Fatal("unknown count became zero")
	}
	o.CandidateBudget = 102
	s.WithOptions(o)
	source.broken = true
	if _, e = s.List(ctx, f.actor, tasks.Filter{}); e == nil {
		t.Fatal("SQL source failure swallowed")
	}
	source.broken = false
	must(t, runtime.ComposeTaskIndexer(f.env.Service.Access()).RestoreEpoch(ctx, "游标契约恢复代次验证"))
	if _, e = s.List(ctx, f.actor, tasks.Filter{Limit: 2, Cursor: first.NextCursor}); e == nil {
		t.Fatal("old epoch cursor accepted")
	}
}

func verifyTaskAvailableResult(t *testing.T, f resourceFixture, ref taskrecord.SourceRef) {
	t.Helper()
	if os.Getenv("INFRAPRO_TASK_ASSERTIONS") != "1" {
		return
	}
	service := runtime.ComposeTasks(f.env.Service.Access())
	v, e := service.Get(ctx, f.actor, ref.ID(), "")
	must(t, e)
	if v.ResultState != "available" {
		t.Fatal("fresh fixed result unavailable", v.Type, v.ResultState)
	}
}
