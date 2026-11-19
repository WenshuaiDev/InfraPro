package identity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// This observation runs only against the frozen source. Its success means
// the native legacy behavior was measured, including the lost update.
func TestArchitectureFrozenSessionActivityObservation(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	s := x.service
	bootstrap(t, s)
	p, token := login(t, s, "Alice", testPassword)
	dbNow := func() time.Time {
		t.Helper()
		var at time.Time
		if err := x.pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	databaseActivity := func() time.Time {
		t.Helper()
		var at time.Time
		if err := x.pool.QueryRow(ctx, "SELECT last_activity_at FROM account_sessions WHERE id=$1", p.Session.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	cached := func() activity {
		t.Helper()
		a, err := s.cache.Activity(ctx, p.Session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	wait := func(until time.Time) {
		t.Helper()
		if until.After(time.Now().Add(20 * time.Second)) {
			t.Fatal("clock outside the supported local interval")
		}
		for time.Now().Before(until) || dbNow().Before(until) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if _, err := x.pool.Exec(ctx, "UPDATE account_sessions SET last_activity_at=clock_timestamp()-interval '119 minutes 50 seconds' WHERE id=$1", p.Session.ID); err != nil {
		t.Fatal(err)
	}
	old := databaseActivity()
	stamp := dbNow().Add(850 * time.Millisecond).Truncate(time.Millisecond)
	if err := s.cache.RecordActivity(ctx, p, stamp, s.cfg.SessionIdle); err != nil {
		t.Fatal(err)
	}
	before := cached()
	if !before.Dirty || !before.At.Equal(stamp) {
		t.Fatal("missing controlled pending observation")
	}
	if !dbNow().Before(stamp) {
		t.Fatal("database already past the initial sample")
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	first := cached()
	firstUnchanged := databaseActivity().Equal(old)
	beforeDue := dbNow().Before(stamp)
	if !beforeDue || first.Dirty || !firstUnchanged {
		t.Fatal("frozen sample did not exhibit the observed pre-due loss")
	}
	wait(stamp.Add(5 * time.Millisecond))
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	second := cached()
	secondUnchanged := databaseActivity().Equal(old)
	warm, err := s.Authenticate(ctx, token)
	if err != nil || !warm.Session.LastActivityAt.Equal(stamp) {
		t.Fatal("authorized warm read did not show the observed activity", err)
	}
	if err := s.ClearSessionCache(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, x.cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	options, err := redis.ParseURL(x.cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	cold, err := NewService(pool, client, x.cfg, s.logger)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := cold.Authenticate(ctx, token)
	if err != nil || !fresh.Session.LastActivityAt.Equal(old) {
		t.Fatal("cold authorized read did not reflect the earlier durable time", err)
	}
	wait(old.Add(s.cfg.SessionIdle + 100*time.Millisecond))
	_, err = cold.Authenticate(ctx, token)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old idle cutoff: %v", err)
	}
	if second.Dirty || !secondUnchanged {
		t.Fatal("frozen second flush unexpectedly recovered an acknowledged observation")
	}
	row, err := json.Marshal(map[string]any{"case": "frozen_future_loss", "lead_ms": 850, "dirty_before": before.Dirty, "dirty_after_first": first.Dirty, "dirty_after_due": second.Dirty, "first_observed_before_due": beforeDue, "postgres_unchanged_before_due": firstUnchanged, "postgres_unchanged_after_due": secondUnchanged, "warm_matches_observation": warm.Session.LastActivityAt.Equal(stamp), "cold_matches_previous": fresh.Session.LastActivityAt.Equal(old), "warm_minus_cold_ms": warm.Session.LastActivityAt.Sub(fresh.Session.LastActivityAt).Milliseconds(), "old_idle_cutoff_denied": true})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("session_checkpoint", string(row))
}
