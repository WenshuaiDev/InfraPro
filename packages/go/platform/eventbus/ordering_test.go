package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func tenant(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	a := access.New(p, slog.Default())
	must(t, a.Sync(ctx))
	acc, e := identity.NewRepository(p).Bootstrap(ctx, "ArcAdmin", "测试", "fixture-no-login", identity.Meta{})
	must(t, e)
	actor := access.Actor{ID: acc.ID, Check: func(context.Context, pgx.Tx, bool) error { return nil }}
	v, e := tenancy.New(a).Create(ctx, actor, tenancy.CreateInput{Fields: tenancy.Fields{Name: "事件验证", Code: "arc_event_test"}, AdministratorID: acc.ID, RequestID: uuid.NewString(), Reason: "构造隔离验证数据"}, access.Meta{})
	must(t, e)
	return v.TenantID
}
func change(ctx context.Context, p *pgxpool.Pool, tenant string, pos int64) (string, error) {
	id := uuid.NewString()
	e := pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_tenant_states(tenant_id,commit_sequence) VALUES($1,$2) ON CONFLICT(tenant_id) DO UPDATE SET commit_sequence=GREATEST(monitoring_tenant_states.commit_sequence,EXCLUDED.commit_sequence)`, tenant, pos); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_changes(id,tenant_id,source_position,schema_version,change_type,source_kind,source_id,impact_hash) VALUES($1,$2,$3,'monitoring-change/1','raw_publish','direct',$4,repeat('0',64))`, id, tenant, pos, uuid.NewString())
		return e
	})
	return id, e
}
func TestContinuousPositionsAndBackpressure(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	scope := tenant(t, p)
	// This baseline already has a legacy commit position before event capture.
	execSQL(t, p, `INSERT INTO monitoring_tenant_states(tenant_id,commit_sequence) VALUES($1,4) ON CONFLICT(tenant_id) DO UPDATE SET commit_sequence=4`, scope)
	state, e := Operate(ctx, p, Operation{Action: "enable", Reason: "freeze cursor in old business version"})
	must(t, e)
	five, e := change(ctx, p, scope, 5)
	must(t, e)
	seven, e := change(ctx, p, scope, 7)
	must(t, e)
	first, later := eventFor(t, p, five), eventFor(t, p, seven)
	c := Defaults()
	c.BackoffMS = 15
	c.AckWaitMS = 1000
	j, e := claimWork(ctx, p, c, "summary", state.Epoch, later.ID)
	must(t, e)
	require(t, j == nil, "position seven overtook five")
	j, e = claimWork(ctx, p, c, "summary", state.Epoch, first.ID)
	must(t, e)
	require(t, j != nil, "first pending position skipped")
	must(t, finishWork(ctx, p, c, *j, nil))
	time.Sleep(25 * time.Millisecond)
	j, e = claimWork(ctx, p, c, "summary", state.Epoch, later.ID)
	must(t, e)
	require(t, j == nil, "missing predecessor silently advanced")
	var reason string
	must(t, p.QueryRow(ctx, `SELECT reason FROM event_quarantine WHERE event_id=$1 AND consumer='summary'`, later.ID).Scan(&reason))
	require(t, reason == "position_gap", "missing durable position gap reason")
	// An independent group still consumes the first position while summary is
	// quarantined. Its checkpoint must not be moved by summary's repair.
	j, e = claimWork(ctx, p, c, "derived", state.Epoch, first.ID)
	must(t, e)
	require(t, j != nil, "independent group unavailable")
	must(t, finishWork(ctx, p, c, *j, nil))
	six, e := change(ctx, p, scope, 6)
	must(t, e)
	middle := eventFor(t, p, six)
	j, e = claimWork(ctx, p, c, "summary", state.Epoch, middle.ID)
	must(t, e)
	require(t, j != nil, "repaired position six unavailable")
	must(t, finishWork(ctx, p, c, *j, nil))
	_, e = Operate(ctx, p, Operation{Action: "redrive", Consumer: "summary", ID: later.ID, Reason: "restored missing source fact"})
	must(t, e)
	j, e = claimWork(ctx, p, c, "summary", state.Epoch, later.ID)
	must(t, e)
	require(t, j != nil, "repaired tail unavailable")
	must(t, finishWork(ctx, p, c, *j, nil))
	var summary, derived int64
	must(t, p.QueryRow(ctx, `SELECT max(position) FILTER(WHERE consumer='summary'),max(position) FILTER(WHERE consumer='derived') FROM event_consumer_positions WHERE scope=$1`, scope).Scan(&summary, &derived))
	require(t, summary == 7 && derived == 5, "consumer positions were not independent")
	_, e = Operate(ctx, p, Operation{Action: "limits", SoftLimit: 2, HardLimit: 3, Reason: "small acceptance capacity"})
	must(t, e)
	rejected, e := change(ctx, p, scope, 8)
	var pe *pgconn.PgError
	require(t, errors.As(e, &pe) && pe.Code == "53300", "hard watermark failed to backpressure new business")
	var exists bool
	must(t, p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_changes WHERE id=$1)`, rejected).Scan(&exists))
	require(t, !exists, "rejected publication became visible")
	// An unrelated auxiliary event is still accepted at the monitoring limit.
	id := uuid.NewString()
	must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
	evidence(t, "ordered-consumers", map[string]any{"scope": scope, "initial_position": 4, "reordered_positions": []int{7, 5}, "gap": reason, "summary": summary, "derived": derived, "hard_limit": 3, "hard_limit_sqlstate": pe.Code, "rejected_visible": exists, "auxiliary_survived": eventFor(t, p, id).ID != ""})
}

func TestMTLSRuntimeFallback(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, e := Operate(ctx, p, Operation{Action: "enable", Reason: "private broker test"})
	must(t, e)
	root := os.Getenv("INFRAPRO_TEST_REPO_ROOT")
	if root == "" {
		root, e = filepath.Abs("../../../../")
		must(t, e)
	}
	dir := t.TempDir()
	cmd := exec.Command("node", filepath.Join(root, "scripts/architecture-event-lab.mjs"), dir, "arcsecure", st.Epoch)
	cmd.Dir = root
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("create isolated PKI: %s %v", b, e)
	}
	var settings struct {
		Port, MonitorPort int
		Config            string
	}
	bytes, e := os.ReadFile(filepath.Join(dir, "lab.json"))
	must(t, e)
	must(t, json.Unmarshal(bytes, &settings))
	node := &node{file: settings.Config}
	node.start(t)
	t.Cleanup(node.stop)
	admin, e := Load(filepath.Join(dir, "event-admin/events.json"))
	must(t, e)
	provisionForTest(t, admin, st.Epoch)
	relay, e := Load(filepath.Join(dir, "event-relay/events.json"))
	must(t, e)
	workerPath := filepath.Join(dir, "notification-worker/events.json")
	worker, e := Load(workerPath)
	must(t, e)
	worker.FallbackScanMS = 250
	worker.BackoffMS = 30
	worker.Batch = 1
	bts, e := json.Marshal(worker)
	must(t, e)
	must(t, os.WriteFile(workerPath, bts, 0600))
	t.Setenv("INFRAPRO_EVENTS_FILE", workerPath)
	runtime, stop := context.WithCancel(ctx)
	done := make(chan error, 2)
	execSQL(t, p, `CREATE TABLE isolated_effects(id uuid PRIMARY KEY,n integer NOT NULL)`)
	limit := p.Config()
	limit.MinConns, limit.MaxConns = 0, 1
	oneConnection, e := pgxpool.NewWithConfig(ctx, limit)
	must(t, e)
	defer oneConnection.Close()
	go func() { done <- RunRelay(runtime, oneConnection, relay, diagnosticreport.Optional(nil)) }()
	go func() {
		done <- RunConsumer(runtime, p, "notification", func(ctx context.Context, v Event) error {
			// Expose a repeated execution, rather than hiding it behind ON CONFLICT DO
			// NOTHING. These fault points have one allowed durable handoff per key.
			_, e := p.Exec(ctx, `INSERT INTO isolated_effects(id,n) VALUES($1,1) ON CONFLICT(id) DO UPDATE SET n=isolated_effects.n+1`, v.AggregateID)
			return e
		})
	}()
	defer func() {
		stop()
		for range 2 {
			select {
			case <-done:
			case <-time.After(7 * time.Second):
				t.Error("event runtime did not stop cleanly")
			}
		}
	}()
	add := func() string {
		id := uuid.NewString()
		must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
		return id
	}
	effect := func(id string) int {
		var n int
		_ = p.QueryRow(ctx, `SELECT n FROM isolated_effects WHERE id=$1`, id).Scan(&n)
		return n
	}
	first := add()
	wait(t, 12*time.Second, func() bool { return effect(first) == 1 }, "live consumer delivery")
	via := func(id string) string {
		var s string
		_ = p.QueryRow(ctx, `SELECT c.via FROM event_consumptions c JOIN event_outbox o USING(restore_epoch,event_id) WHERE o.aggregate_id=$1`, id).Scan(&s)
		return s
	}
	wait(t, 5*time.Second, func() bool {
		var ok bool
		_ = p.QueryRow(ctx, `SELECT state='sent' FROM event_outbox WHERE aggregate_id=$1`, first).Scan(&ok)
		return ok
	}, "publish confirmation over mTLS")
	// Wait for the process to have subscribed before asserting a direct hint.
	// PG recovery of a work item committed before subscribe is a correct outcome.
	second := add()
	wait(t, 5*time.Second, func() bool { return effect(second) == 1 }, "direct broker delivery")
	require(t, via(second) == "broker", "normal delivery did not use the authorized broker path")
	deny := worker
	deny.CertFile = ""
	deny.KeyFile = "" // TLS without a client identity
	// NATS uses a TLS URL and RootCAs, so the missing client certificate must
	// prevent connection even before consumer ACL evaluation.
	_, e = connect(deny, st.Epoch, "notification")
	require(t, e != nil, "uncredentialed broker client accepted")
	wrong, e := connect(worker, st.Epoch, "notification")
	must(t, e)
	req, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	_, e = wrong.js.Publish(req, wrong.subject("monitoring.changed"), []byte("{}"))
	cancel()
	wrong.nc.Close()
	require(t, e != nil, "consumer certificate published outside its ACL")
	node.stop()
	third := add()
	wait(t, 5*time.Second, func() bool { return effect(third) == 1 }, "broker offline PG fallback")
	require(t, via(third) == "pg", "offline recovery did not use PG")
	node.start(t)
	wait(t, 15*time.Second, func() bool {
		var ok bool
		_ = p.QueryRow(ctx, `SELECT state='sent' FROM event_outbox WHERE aggregate_id=$1`, third).Scan(&ok)
		return ok
	}, "broker restarted and retained outbox caught up")
	wait(t, 5*time.Second, func() bool {
		var n int
		_ = p.QueryRow(ctx, `SELECT sum(c.duplicates) FROM event_consumptions c JOIN event_outbox o USING(restore_epoch,event_id) WHERE o.aggregate_id=$1`, third).Scan(&n)
		return n > 0
	}, "broker replay after offline fallback")
	require(t, effect(third) == 1, "broker recovery executed duplicate effect")
	evidence(t, "mtls-fallback", map[string]any{"normal": via(second), "offline": via(third), "effect_after_replay": effect(third), "uncredentialed_rejected": true, "wrong_subject_rejected": true})
}
