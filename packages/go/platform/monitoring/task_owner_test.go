package monitoring

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ownerTestExecutor struct{}

func (ownerTestExecutor) AuthorizeExecutionTx(context.Context, pgx.Tx, Task) error { return nil }
func (ownerTestExecutor) AuthorizeTaskTx(context.Context, pgx.Tx, Task, access.Actor, Scope, bool) error {
	return nil
}
func (ownerTestExecutor) ExecuteTask(context.Context, TaskLease) error { return nil }

func TestArchitectureEligibleTaskOwnersAndIdleSchedulers(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("isolated PostgreSQL fixture required")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(env.Close)
	const pw = "Arch.Owner.Integration.8B!2026"
	if _, e = env.Service.Bootstrap(ctx, "ArcOwner", "架构验收", pw, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	_, session, _, e := env.Service.Login(ctx, "ArcOwner", pw, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	p, e := env.Service.Authenticate(ctx, session)
	if e != nil {
		t.Fatal(e)
	}
	owner := env.FixtureActor(p)
	tenant, e := tenancy.New(env.Service.Access()).Create(ctx, owner, tenancy.CreateInput{Fields: tenancy.Fields{Name: "角色验收", Code: "arc-owner"}, AdministratorID: owner.ID, RequestID: uuid.NewString(), Reason: "验证分工"}, access.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	host := New(env.Service.Access()).WithStorage(new(Storage))
	executors := map[string]TaskExecutor{}
	for _, kind := range calculationKinds {
		executors[kind] = ownerTestExecutor{}
	}
	_, e = host.WithTaskExecutors(executors)
	if e != nil {
		t.Fatal(e)
	}
	fixtures := []struct {
		kind, input string
		role        WorkerRole
		person      bool
	}{
		{"validate", "{}", PublishWorker, true}, {"publish", "{}", PublishWorker, true}, {"relocate", "{}", PublishWorker, false}, {"verify", "{}", PublishWorker, false},
		{"summary", `{"mode":"maintenance"}`, IncrementalWorker, false}, {"summary", `{"mode":"query"}`, ReadWorker, true}, {"summary", `{"mode":"rebuild"}`, HistoryWorker, false},
		{"derive", `{"mode":"automatic"}`, IncrementalWorker, false}, {"derive", `{"mode":"correction"}`, IncrementalWorker, false}, {"derive", `{"mode":"trial"}`, IncrementalWorker, true},
		{"derive", `{"mode":"backfill"}`, HistoryWorker, true}, {"derive", `{"mode":"historical_replay"}`, HistoryWorker, true},
		{"analysis", `{"trial":true}`, IncrementalWorker, true}, {"analysis", `{"trial":false}`, HistoryWorker, true}, {"analysis", "{}", HistoryWorker, true},
		{"derived_replay", "{}", HistoryWorker, true}, {"analysis_compare", "{}", HistoryWorker, true}, {"export", "{}", HistoryWorker, true},
		{"analysis_export", "{}", HistoryWorker, true}, {"derived_evidence_export", "{}", HistoryWorker, true}, {"summary", `{"mode":"unsupported"}`, HistoryWorker, false},
	}
	roles := []WorkerRole{PublishWorker, IncrementalWorker, HistoryWorker, ReadWorker}
	for _, v := range fixtures {
		t.Run(v.kind+"/"+v.input, func(t *testing.T) {
			id := uuid.NewString()
			var actor any
			if v.person {
				actor = owner.ID
			}
			_, e := env.Pool.Exec(ctx, `INSERT INTO monitoring_jobs(id,tenant_id,actor_id,kind,target_id,target_revision,input_hash,input) VALUES($1,$2,$3,$4,$1,1,repeat('0',64),$5)`, id, tenant.TenantID, actor, v.kind, v.input)
			if e != nil {
				t.Fatal(e)
			}
			got, e := host.JobOwner(ctx, tenant.TenantID, id)
			if e != nil || got != v.role {
				t.Fatal(got, e)
			}
			for _, other := range roles {
				if other == v.role {
					continue
				}
				did, e := host.RunTask(WorkerContext(ctx, other), "other", tenant.TenantID, id)
				if e != nil || did {
					t.Fatalf("unrelated role %s claimed job: %t %v", other, did, e)
				}
			}
			if v.input == `{"mode":"unsupported"}` {
				for attempt := 1; attempt <= host.workers.MaxAttempts; attempt++ {
					claimed, failure := host.RunTask(WorkerContext(ctx, v.role), "explicit-cli", tenant.TenantID, id)
					if !claimed || failure == nil {
						t.Fatal("malformed input did not fail in the owning executor", claimed, failure)
					}
					var state, code string
					var future bool
					var n int
					if e := env.Pool.QueryRow(ctx, `SELECT state,error_code,attempts,available_at>clock_timestamp() FROM monitoring_jobs WHERE id=$1`, id).Scan(&state, &code, &n, &future); e != nil {
						t.Fatal(e)
					}
					if n != attempt || code == "" {
						t.Fatal("missing terminal reason or lost attempt", n, code)
					}
					if attempt == host.workers.MaxAttempts {
						if state != "failed" {
							t.Fatal("invalid input never reached a terminal state", state)
						}
					} else {
						if state != "retry_wait" || !future {
							t.Fatal("invalid input formed a hot loop", state, future)
						}
						claimed, e := host.RunTask(WorkerContext(ctx, v.role), "immediate-retry", tenant.TenantID, id)
						if claimed || e != nil {
							t.Fatal("retry ignored backoff", claimed, e)
						}
						if _, e = env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1`, id); e != nil {
							t.Fatal(e)
						}
					}
				}
				return
			}
			// CLI resolves JobOwner above, while workers additionally use the queue.
			j, ok, e := host.claimJob(WorkerContext(ctx, v.role), "selected", tenant.TenantID, id, "")
			if e != nil || !ok || j.Kind != v.kind || j.State != "running" {
				t.Fatalf("%s could not claim: %+v %t %v", v.role, j, ok, e)
			}
			again, ok, e := host.claimJob(WorkerContext(ctx, v.role), "another", tenant.TenantID, id, "")
			if e != nil || ok {
				t.Fatal("live lease granted to a second worker", again, e)
			}
			if _, e = env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET state='cancelled',lease_until=NULL,lease_owner=NULL WHERE id=$1`, id); e != nil {
				t.Fatal(e)
			}
		})
	}
	// Two replicas race to service the same due clock with no messages.
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- env.Service.Access().RunScheduled(ctx, "arc-clock", time.Minute, func(context.Context) error { close(started); <-release; return nil })
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("clock timeout")
	}
	unexpected := errors.New("duplicate scheduler callback")
	duplicate := func(context.Context) error { return unexpected }
	if e = env.Service.Access().RunScheduled(ctx, "arc-clock", time.Minute, duplicate); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if e = env.Service.Access().RunScheduled(ctx, "arc-clock", time.Minute, duplicate); e != nil {
		t.Fatal(e)
	}
	if _, e = env.Pool.Exec(ctx, `UPDATE monitoring_scheduler_leases SET next_run=clock_timestamp()-interval '1 second' WHERE lane='arc-clock'`); e != nil {
		t.Fatal(e)
	}
	if e = env.Service.Access().RunScheduled(ctx, "arc-clock", time.Minute, duplicate); !errors.Is(e, unexpected) {
		t.Fatal("restarted clock did not pick up due work", e)
	}
}
