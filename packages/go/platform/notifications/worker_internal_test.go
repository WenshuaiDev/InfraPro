package notifications

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestExpiredClaimFencingPoisonIsolationAndReplay(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:notifications")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if _, e = env.Service.Bootstrap(ctx, "LeaseOwner", "租约测试", "Lease river 河流 2026!", identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	var account string
	if e = env.Pool.QueryRow(ctx, `SELECT id::text FROM accounts WHERE username='LeaseOwner'`).Scan(&account); e != nil {
		t.Fatal(e)
	}
	s := New(access.New(env.Pool, slog.Default()), nil)
	emit := func() string {
		t.Helper()
		var id string
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			var e error
			id, e = ne.AppendTx(ctx, tx, ne.Event{Module: "lease_test", Key: uuid.NewString(), Type: "security.changed", Scope: ne.Scope{Kind: "account"}, Target: ne.Target{Kind: "security"}, Outcome: "changed", Recipients: ne.Personal(account)})
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	first := emit()
	old, e := s.claim(ctx)
	if e != nil || len(old) != 1 {
		t.Fatal(old, e)
	}
	// The process dies after its claim commits. A new owner must fence it out.
	if _, e = env.Pool.Exec(ctx, `UPDATE notification_deliveries SET lease_until=clock_timestamp()-interval '1 second' WHERE event_id=$1`, first); e != nil {
		t.Fatal(e)
	}
	next, e := s.claim(ctx)
	if e != nil || len(next) != 1 {
		t.Fatal(next, e)
	}
	if e = s.deliver(ctx, old[0]); !errors.Is(e, errFence) {
		t.Fatal("stale owner committed", e)
	}
	if e = s.deliver(ctx, next[0]); e != nil {
		t.Fatal(e)
	}
	if e = s.failure(ctx, old[0], errors.New("late failure")); e != nil {
		t.Fatal(e)
	}
	var state string
	if e = env.Pool.QueryRow(ctx, `SELECT state FROM notification_deliveries WHERE event_id=$1`, first).Scan(&state); e != nil || state != "delivered" {
		t.Fatal(state, e)
	}
	poison, healthy := emit(), emit()
	if _, e = env.Pool.Exec(ctx, `UPDATE notification_events SET event_type='future.unsupported' WHERE id=$1`, poison); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Tick(ctx); e == nil {
		t.Fatal("unknown schema was silently accepted")
	}
	if e = env.Pool.QueryRow(ctx, `SELECT state FROM notification_deliveries WHERE event_id=$1`, healthy).Scan(&state); e != nil || state != "delivered" {
		t.Fatal("poison blocked independent fact", state, e)
	}
	if e = env.Pool.QueryRow(ctx, `SELECT state FROM notification_deliveries WHERE event_id=$1`, poison).Scan(&state); e != nil || state != "failed" {
		t.Fatal(state, e)
	}
	if _, e = env.Pool.Exec(ctx, `UPDATE notification_events SET event_type='security.changed' WHERE id=$1`, poison); e != nil {
		t.Fatal(e)
	}
	if n, e := s.Replay(ctx, poison, "修复测试模板后安全重放"); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	if n, e := s.Replay(ctx, first, "确认已投递事件不重复重放"); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	var n int
	if e = env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_notifications WHERE account_id=$1`, account).Scan(&n); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	// Heartbeat depends only on PostgreSQL and remains observable with no work.
	run, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Run(run, "lease-test", func(e error) { t.Error(e) }) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		d, e := s.Diagnostics(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if d.LastHeartbeat != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat absent")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
	if e = s.CheckWorker(ctx, time.Minute); e != nil {
		t.Fatal(e)
	}
	if _, e = env.Pool.Exec(ctx, `UPDATE notification_worker_heartbeats SET heartbeat_at=clock_timestamp()-interval '2 minutes'`); e != nil {
		t.Fatal(e)
	}
	if e = s.CheckWorker(ctx, time.Minute); e == nil {
		t.Fatal("stale worker reported healthy")
	}
}
