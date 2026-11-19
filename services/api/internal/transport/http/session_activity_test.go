package http_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func TestActivityHTTPRespectsSessionLifetime(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires the isolated account PostgreSQL and Redis")
	}
	ctx := context.Background()
	env, err := testsupport.New(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	if _, err = env.Service.Bootstrap(ctx, "ActivityAdmin", "活动核验", password, identity.Meta{}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	for _, kind := range []string{"active_before_idle", "already_idle", "absolute_limit", "revoked"} {
		t.Run(kind, func(t *testing.T) {
			client := newMFAHTTPClient(t, server, env.Config.TrustedOrigins[0])
			start := time.Now()
			response, _ := client.call("POST", "/auth/login", map[string]string{"username": "ActivityAdmin", "password": password}, 200)
			end := time.Now()
			var token string
			for _, c := range response.Cookies() {
				if c.Name == "infrapro_session" {
					token = c.Value
				}
			}
			if token == "" {
				t.Fatal("missing session cookie")
			}
			hash := sha256.Sum256([]byte(token))
			var id string
			var expiry time.Time
			if err := env.Pool.QueryRow(ctx, "SELECT id::text,expires_at FROM account_sessions WHERE token_hash=$1", hex.EncodeToString(hash[:])).Scan(&id, &expiry); err != nil {
				t.Fatal(err)
			}
			if expiry.Before(start.Add(12*time.Hour-time.Millisecond)) || expiry.After(end.Add(12*time.Hour+time.Millisecond)) {
				t.Fatalf("issued session exceeded its 12-hour login bounds: %v", expiry)
			}
			client.call("GET", "/account/sessions", nil, 200)
			key := env.Config.RedisKeyPrefix + "activity:" + id
			readTimes := func() (time.Time, time.Time) {
				t.Helper()
				var activity, expires time.Time
				if err := env.Pool.QueryRow(ctx, "SELECT last_activity_at,expires_at FROM account_sessions WHERE id=$1", id).Scan(&activity, &expires); err != nil {
					t.Fatal(err)
				}
				return activity, expires
			}
			update := func(sql string) {
				t.Helper()
				if _, err := env.Pool.Exec(ctx, sql, id); err != nil {
					t.Fatal(err)
				}
			}
			sample := map[string]any{"case": "http_" + kind, "original_absolute_hours": 12, "idle_hours": 2}
			if kind == "active_before_idle" {
				update("UPDATE account_sessions SET last_activity_at=clock_timestamp()-interval '2 hours'+interval '5 seconds' WHERE id=$1")
				previous, _ := readTimes()
				client.call("POST", "/auth/activity", nil, 204)
				fields, err := env.Redis.HMGet(ctx, key, "at", "dirty").Result()
				if err != nil || fields[1] != "1" {
					t.Fatalf("authenticated activity was not pending: %v", err)
				}
				ms, err := strconv.ParseInt(fields[0].(string), 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				at := time.UnixMilli(ms)
				if !at.Before(previous.Add(2 * time.Hour)) {
					t.Fatal("fixture missed the original active interval")
				}
				// Let the old durable value cross two hours while the already verified
				// Redis observation keeps the session active. Both clocks must be due.
				until := time.Now().Add(12 * time.Second)
				for {
					var dbTime time.Time
					if err := env.Pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&dbTime); err != nil {
						t.Fatal(err)
					}
					if dbTime.After(previous.Add(2*time.Hour)) && !dbTime.Before(at) && !time.Now().Before(at) {
						break
					}
					if time.Now().After(until) {
						t.Fatal("isolated database clock did not reach the recorded activity")
					}
					time.Sleep(20 * time.Millisecond)
				}
				client.call("GET", "/account/sessions", nil, 200)
				if err := env.Service.Flush(ctx); err != nil {
					t.Fatal(err)
				}
				stored, afterExpiry := readTimes()
				if !stored.Equal(at) || !afterExpiry.Equal(expiry) {
					t.Fatal("flush changed the session lifetime or lost confirmed activity")
				}
				if err := env.Service.ClearSessionCache(ctx); err != nil {
					t.Fatal(err)
				}
				client.call("GET", "/account/sessions", nil, 200)
				sample["activity_status"], sample["hot_read_status"], sample["after_cache_clear_status"] = 204, 200, 200
				sample["reached_old_idle_cutoff"], sample["durable_matches_verified"], sample["absolute_unchanged"] = true, stored.Equal(at), afterExpiry.Equal(expiry)
			} else {
				switch kind {
				case "already_idle":
					update("UPDATE account_sessions SET last_activity_at=clock_timestamp()-interval '2 hours 1 second' WHERE id=$1")
				case "absolute_limit":
					update("UPDATE account_sessions SET created_at=clock_timestamp()-interval '12 hours 1 second', expires_at=clock_timestamp()-interval '1 second', last_activity_at=clock_timestamp()-interval '1 minute' WHERE id=$1")
				case "revoked":
					client.call("DELETE", "/account/sessions/"+id, nil, 204)
				}
				before, initialExpiry := readTimes()
				_, body := client.call("POST", "/auth/activity", nil, 401)
				if body["code"] != "unauthenticated" {
					t.Fatalf("activity rejection %v", body["code"])
				}
				activityKeys, err := env.Redis.Exists(ctx, key).Result()
				if err != nil || activityKeys != 0 {
					t.Fatalf("rejected activity created a pending marker: %v", err)
				}
				if err := env.Service.Flush(ctx); err != nil {
					t.Fatal(err)
				}
				after, finalExpiry := readTimes()
				if !after.Equal(before) || !finalExpiry.Equal(initialExpiry) {
					t.Fatal("rejected activity revived or extended a session")
				}
				client.call("GET", "/account/sessions", nil, 401)
				sample["activity_status"], sample["read_after_flush_status"], sample["activity_cache_keys"] = 401, 401, activityKeys
				sample["durable_unchanged"], sample["absolute_unchanged"] = after.Equal(before), finalExpiry.Equal(initialExpiry)
			}
			encoded, err := json.Marshal(sample)
			if err != nil {
				t.Fatal(err)
			}
			t.Log("session_checkpoint", string(encoded))
		})
	}
}
