package identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type activityWriteTrace struct {
	writes     int
	afterFirst func()
}
type activityWriteKey struct{}

func (t *activityWriteTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, activityWriteKey{}, strings.Contains(q.SQL, "UPDATE account_sessions s SET last_activity_at"))
}

func (t *activityWriteTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if relevant, _ := ctx.Value(activityWriteKey{}).(bool); !relevant {
		return
	}
	t.writes++
	if t.writes == 1 && t.afterFirst != nil {
		t.afterFirst()
	}
}

// Only the generated fixture schema shadows PostgreSQL now(). The authenticated
// API still records its native time. Advancing the private clock after the first
// SQL decision makes both transactions observable independently of host load.
func TestSessionActivityFlushUsesDatabaseTime(t *testing.T) {
	x := setup(t)
	svc, ctx := x.service, context.Background()
	bootstrap(t, svc)
	original := svc.repo
	var schema string
	if err := x.pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	execute := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := x.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	execute(t, "CREATE TABLE activity_test_clock (at timestamptz NOT NULL)")
	execute(t, "INSERT INTO activity_test_clock VALUES (pg_catalog.now())")
	execute(t, "CREATE FUNCTION "+quoted+".now() RETURNS timestamptz LANGUAGE sql STABLE AS 'SELECT at FROM "+quoted+".activity_test_clock'")
	config := x.pool.Config()
	config.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	trace := &activityWriteTrace{}
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	setClock := func(t *testing.T, at time.Time) { t.Helper(); execute(t, "UPDATE activity_test_clock SET at=$1", at) }
	readStored := func(t *testing.T, id string) time.Time {
		t.Helper()
		var at time.Time
		if err := x.pool.QueryRow(ctx, "SELECT last_activity_at FROM account_sessions WHERE id=$1", id).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	readCache := func(t *testing.T, id string) activity {
		t.Helper()
		a, err := svc.cache.Activity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	owner, _ := login(t, svc, "Alice", testPassword)
	invite(t, svc, owner, "Bob")

	for _, kind := range []string{"trusted", "large_skew", "legacy_cache", "missing_marker", "scheduled", "cancelled", "revoked", "expired", "old_observation", "newer_activity", "same_time_new_version", "stale_version"} {
		t.Run(kind, func(t *testing.T) {
			svc.repo = original
			account := "Alice"
			if kind == "stale_version" {
				account = "Bob"
			}
			p, _ := login(t, svc, account, testPassword)
			id := p.Session.ID
			execute(t, "UPDATE account_sessions SET last_activity_at=now()-interval '119 minutes' WHERE id=$1", id)
			old := readStored(t, id)
			if err := svc.Activity(ctx, p); err != nil {
				t.Fatal(err)
			}
			at := readCache(t, id).At
			if kind == "legacy_cache" {
				// A former writer may replace at and leave service_at from a newer binary.
				key := svc.cache.prefix + "activity:" + id
				if err := x.redis.HSet(ctx, key, "service_at", at.Add(-time.Second).UnixMilli()).Err(); err != nil {
					t.Fatal(err)
				}
			} else if kind == "missing_marker" {
				if err := x.redis.HDel(ctx, svc.cache.prefix+"activity:"+id, "service_at").Err(); err != nil {
					t.Fatal(err)
				}
			}
			lag := 7 * time.Millisecond
			if kind == "large_skew" {
				lag = 850 * time.Millisecond
			} else if kind == "old_observation" {
				lag = -svc.cfg.SessionIdle
			}
			setClock(t, at.Add(-lag))
			svc.repo = NewRepository(pool)
			defer func() { svc.repo = original }()
			useCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			trace.writes = 0
			trace.afterFirst = func() {
				// A second conditional UPDATE must see both the newly due time and
				// any concurrently committed session or cache change.
				switch kind {
				case "large_skew", "legacy_cache", "missing_marker", "scheduled", "old_observation":
					return
				case "cancelled":
					cancel()
					return
				case "revoked":
					execute(t, "UPDATE account_sessions SET revoked_at=now() WHERE id=$1", id)
				case "expired":
					execute(t, "UPDATE account_sessions SET expires_at=$2 WHERE id=$1", id, at)
				case "stale_version":
					execute(t, "UPDATE accounts SET auth_version=auth_version+1 WHERE id=$1", p.Account.ID)
				case "newer_activity":
					if err := svc.cache.RecordActivity(ctx, p, at.Add(time.Second), svc.cfg.SessionIdle); err != nil {
						t.Fatal(err)
					}
				case "same_time_new_version":
					if err := svc.cache.Delete(ctx, id, p.TokenHash); err != nil {
						t.Fatal(err)
					}
					replacement := p
					replacement.Version++
					if err := svc.cache.RecordActivity(ctx, replacement, at, svc.cfg.SessionIdle); err != nil {
						t.Fatal(err)
					}
				}
				setClock(t, at)
			}
			if kind == "scheduled" {
				err = svc.cache.FlushActivities(useCtx, svc.repo, svc.cfg.SessionIdle)
			} else {
				err = svc.Flush(useCtx)
			}
			if kind == "cancelled" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("flush after cancellation: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			first := readStored(t, id)
			pending := readCache(t, id)
			firstWrites := trace.writes
			switch kind {
			case "large_skew", "legacy_cache", "missing_marker", "scheduled", "cancelled":
				if !first.Equal(old) || !pending.Dirty || trace.writes != 1 {
					t.Fatalf("future activity: stored=%v dirty=%t writes=%d", first, pending.Dirty, trace.writes)
				}
				trace.afterFirst = nil
				setClock(t, at)
				if err := svc.Flush(ctx); err != nil {
					t.Fatal(err)
				}
				if !readStored(t, id).Equal(at) || readCache(t, id).Dirty {
					t.Fatal("eligible observation was lost")
				}
			case "revoked", "expired", "stale_version":
				if !first.Equal(old) || pending.Dirty || trace.writes != 2 {
					t.Fatalf("invalidated activity: stored=%v dirty=%t writes=%d", first, pending.Dirty, trace.writes)
				}
			case "old_observation":
				if !first.Equal(old) || pending.Dirty || trace.writes != 1 {
					t.Fatal("activity at the two-hour idle cutoff was written")
				}
			case "same_time_new_version":
				if !first.Equal(at) || !pending.At.Equal(at) || !pending.Dirty || pending.Version != p.Version+1 {
					t.Fatal("acknowledging prior activity erased a replacement with the same timestamp and a different version")
				}
				if err := svc.Flush(ctx); err != nil {
					t.Fatal(err)
				}
				if readCache(t, id).Dirty || !readStored(t, id).Equal(at) {
					t.Fatal("a replacement carrying the wrong database version changed the durable activity")
				}
			case "newer_activity":
				if !first.Equal(at) || !pending.Dirty || !pending.At.Equal(at.Add(time.Second)) || trace.writes != 2 {
					t.Fatal("finishing prior activity erased a new observation")
				}
				setClock(t, pending.At)
				if err := svc.Flush(ctx); err != nil {
					t.Fatal(err)
				}
				if !readStored(t, id).Equal(pending.At) || readCache(t, id).Dirty {
					t.Fatal("newer observation did not persist when due")
				}
			default:
				if !first.Equal(at) || pending.Dirty || trace.writes != 2 {
					t.Fatalf("explicit flush: stored=%v dirty=%t writes=%d", first, pending.Dirty, trace.writes)
				}
			}
			row, err := json.Marshal(map[string]any{
				"case": "clock_" + kind, "observation_minus_pg_us": lag.Microseconds(), "initial_conditional_writes": firstWrites,
				"persisted_on_return": first.Equal(at), "dirty_on_return": pending.Dirty, "initial_value_unchanged": first.Equal(old),
				"pending_version_matches": pending.Version == p.Version, "durable_matches_observation_after_due": readStored(t, id).Equal(at),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Log("session_checkpoint", string(row))
		})
	}
}

func TestSessionActivityKeepsIdleAndAbsoluteLimits(t *testing.T) {
	x := setup(t)
	svc, ctx := x.service, context.Background()
	bootstrap(t, svc)
	start := time.Now()
	p, token := login(t, svc, "Alice", testPassword)
	end := time.Now()
	expires := p.Session.ExpiresAt
	if expires.Before(start.Add(12*time.Hour-time.Millisecond)) || expires.After(end.Add(12*time.Hour+time.Millisecond)) {
		t.Fatalf("login absolute expiry %v outside its 12-hour interval", expires)
	}
	update := func(sql string) {
		t.Helper()
		if _, err := x.pool.Exec(ctx, sql, p.Session.ID); err != nil {
			t.Fatal(err)
		}
	}
	update("UPDATE account_sessions SET last_activity_at=now()-interval '119 minutes' WHERE id=$1")
	if err := svc.ClearSessionCache(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Activity(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearSessionCache(ctx); err != nil {
		t.Fatal(err)
	}
	p, err = svc.Authenticate(ctx, token)
	if err != nil || time.Since(p.Session.LastActivityAt) > time.Minute || !p.Session.ExpiresAt.Equal(expires) {
		t.Fatalf("recent activity after cache loss: session=%+v error=%v", p.Session, err)
	}
	update("UPDATE account_sessions SET last_activity_at=now()-interval '2 hours 1 second' WHERE id=$1")
	if err := svc.ClearSessionCache(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Authenticate(ctx, token)
	requireCode(t, err, "unauthenticated")
	p, token = login(t, svc, "Alice", testPassword)
	update("UPDATE account_sessions SET created_at=now()-interval '12 hours 1 second', expires_at=now()-interval '1 second', last_activity_at=now()-interval '1 minute' WHERE id=$1")
	if err := svc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Authenticate(ctx, token)
	requireCode(t, err, "unauthenticated")
	t.Log("session_checkpoint", `{"case":"limits","idle_hours":2,"absolute_hours":12,"recent_durable_after_cache_clear":true,"idle_denied":true,"absolute_denied":true,"renewal_kept_initial_absolute_expiry":true}`)
}
