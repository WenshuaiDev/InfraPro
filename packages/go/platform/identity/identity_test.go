package identity

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const testPassword = "Green bridge 河流 2026!"

func TestPasswordRules(t *testing.T) {
	for _, tc := range []struct {
		password string
		want     bool
	}{
		{"ab中🙂xy7", false}, {"ab中🙂xy78", true}, {strings.Repeat("中🙂", 64), true}, {strings.Repeat("中🙂", 64) + "a", false}, {"password", false}, {"        ", false}, {"aaaaaaaa", false}, {"  pass 汉字  ", true},
	} {
		if got := ValidatePassword(tc.password) == nil; got != tc.want {
			t.Errorf("length %d accepted=%v want %v", len([]rune(tc.password)), got, tc.want)
		}
	}
	hash, e := HashPassword("  pass 汉字  ")
	if e != nil {
		t.Fatal(e)
	}
	if !VerifyPassword(hash, "  pass 汉字  ") || VerifyPassword(hash, "pass 汉字") || VerifyPassword(hash, testPassword) {
		t.Fatal("hash verification changed password bytes")
	}
	if strings.Contains(hash, "pass") || !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatal("unexpected stored password")
	}
}
func BenchmarkPasswordHash(b *testing.B) {
	for b.Loop() {
		if _, e := HashPassword(testPassword); e != nil {
			b.Fatal(e)
		}
	}
}

type testSystem struct {
	service *Service
	pool    *pgxpool.Pool
	redis   *redis.Client
	cfg     config.Config
}

func setup(t *testing.T) *testSystem {
	t.Helper()
	dbURL := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	redisURL := os.Getenv("INFRAPRO_TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("real PostgreSQL/Redis integration: run pnpm test:identity")
	}
	u, e := url.Parse(dbURL)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		t.Fatal(e)
	}
	token, e := newToken()
	if e != nil {
		t.Fatal(e)
	}
	schema := "identity_test_" + digest(token)[:16]
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dbURL = u.String()
	if e = migrations.Up(ctx, dbURL); e != nil {
		t.Fatal(e)
	}
	pool, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	if e = access.New(pool, slog.Default()).Sync(ctx); e != nil {
		t.Fatal(e)
	}
	options, e := redis.ParseURL(redisURL)
	if e != nil {
		t.Fatal(e)
	}
	options.MaxRetries = 0
	options.ReadTimeout = 100 * time.Millisecond
	options.WriteTimeout = 100 * time.Millisecond
	cache := redis.NewClient(options)
	t.Cleanup(func() { cache.Close() })
	cfg := config.Config{DatabaseURL: dbURL, RedisURL: redisURL, RedisKeyPrefix: "infrapro:test-" + schema + ":identity:v1:", PublicConsoleURL: "http://127.0.0.1:15173/", TrustedOrigins: []string{"http://127.0.0.1:15173"}, SupportContact: "测试维护员", CSRFKey: []byte("0123456789abcdef0123456789abcdef"), SessionIdle: 2 * time.Hour, SessionAbsolute: 12 * time.Hour, SessionCacheTTL: 5 * time.Minute, InvitationTTL: 24 * time.Hour, ResetTTL: 30 * time.Minute, AuditRetention: 90 * 24 * time.Hour, InvitationRate: 100, InvitationLimit: 20, IPRequestLimit: 10000, CredentialRequestLimit: 1000, PasswordFailures: 5}
	s, e := NewService(pool, cache, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		var cursor uint64
		for {
			keys, next, e := cache.Scan(ctx, cursor, cfg.RedisKeyPrefix+"*", 100).Result()
			if e != nil {
				break
			}
			if len(keys) > 0 {
				_ = cache.Del(ctx, keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	})
	return &testSystem{s, pool, cache, cfg}
}
func login(t *testing.T, s *Service, username, password string) (Principal, string) {
	t.Helper()
	ctx := context.Background()
	_, token, _, e := s.Login(ctx, username, password, Meta{IP: "127.0.0.1", UserAgent: "test browser"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	return p, token
}
func requireCode(t *testing.T, e error, code string) {
	t.Helper()
	if e == nil || PublicError(e).Code != code {
		t.Fatalf("got %v, want %s", e, code)
	}
}
func linkToken(link Link) string {
	u, _ := url.Parse(link.URL)
	v, _ := url.ParseQuery(u.Fragment)
	return v.Get("token")
}
func bootstrap(t *testing.T, s *Service) {
	t.Helper()
	if _, e := s.Bootstrap(context.Background(), "Alice", "甲", testPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
}
func invite(t *testing.T, s *Service, p Principal, user string) {
	t.Helper()
	l, e := s.CreateInvitation(context.Background(), p, "", "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AcceptInvitation(context.Background(), linkToken(l), user, "", testPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
}

func TestIntegrationBootstrapAndInvitationRaces(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Go(func() { _, e := s.Bootstrap(ctx, "Alice", "甲", testPassword, Meta{}); errs <- e })
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			requireCode(t, e, "already_initialized")
		}
	}
	if success != 1 {
		t.Fatalf("bootstrap successes %d", success)
	}
	p, _ := login(t, s, "aLiCe", testPassword)
	l, e := s.CreateInvitation(ctx, p, "", "项目同事", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	token := linkToken(l)
	for i := 0; i < 2; i++ {
		if _, e = s.Inspect(ctx, token, false, Meta{}); e != nil {
			t.Fatal(e)
		}
	}
	requireCode(t, s.AcceptInvitation(ctx, token, "Alice", "", testPassword, Meta{}), "username_taken")
	requireCode(t, s.AcceptInvitation(ctx, token, "Bob", "", "password", Meta{}), "validation_failed")
	errs = make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Go(func() { errs <- s.AcceptInvitation(ctx, token, "Bob", "乙", testPassword, Meta{}) })
	}
	wg.Wait()
	close(errs)
	success = 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			requireCode(t, e, "invalid_link")
		}
	}
	if success != 1 {
		t.Fatalf("accept successes %d", success)
	}
	_, e = s.Inspect(ctx, token, false, Meta{})
	requireCode(t, e, "invalid_link")
	// Different credentials competing for the same normalized username only consume one invitation.
	l1, e := s.CreateInvitation(ctx, p, "", "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	l2, e := s.CreateInvitation(ctx, p, "", "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	errs = make(chan error, 2)
	for _, link := range []Link{l1, l2} {
		wg.Go(func() { errs <- s.AcceptInvitation(ctx, linkToken(link), "Charlie", "", testPassword, Meta{}) })
	}
	wg.Wait()
	close(errs)
	success = 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			requireCode(t, e, "username_taken")
		}
	}
	if success != 1 {
		t.Fatal("username unique invariant")
	}
	var pending int
	if e = x.pool.QueryRow(ctx, "SELECT count(*) FROM account_invitations WHERE used_at IS NULL AND revoked_at IS NULL").Scan(&pending); e != nil || pending != 1 {
		t.Fatalf("losing invitation consumed: %d %v", pending, e)
	}
	// Invited accounts receive only member capabilities until explicitly authorized.
	b, _ := login(t, s, "Bob", testPassword)
	_, e = s.CreateInvitation(ctx, b, "", "", Meta{})
	requireCode(t, e, "permission_denied")
}
func TestIntegrationSessionRevocationAndRecovery(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	a, token := login(t, s, "Alice", testPassword)
	a2, token2 := login(t, s, "Alice", testPassword)
	cacheKey := x.cfg.RedisKeyPrefix + "session:" + digest(token)
	raw, e := x.redis.Get(ctx, cacheKey).Result()
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(raw, token) || strings.Contains(raw, testPassword) {
		t.Fatal("secret in cache")
	}
	ttl := x.redis.TTL(ctx, cacheKey).Val()
	if ttl <= 0 || ttl > x.cfg.SessionCacheTTL {
		t.Fatalf("TTL %s", ttl)
	}
	if e = x.redis.Del(ctx, cacheKey).Err(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, token); e != nil {
		t.Fatal(e)
	}
	if x.redis.Exists(ctx, cacheKey).Val() != 1 {
		t.Fatal("miss did not refill")
	}
	restarted, e := NewService(x.pool, x.redis, x.cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	after, e := restarted.Authenticate(ctx, token)
	if e != nil || !after.Session.ExpiresAt.Equal(a.Session.ExpiresAt) {
		t.Fatal("restart changed session deadline", e)
	}
	if e = s.Revoke(ctx, a, a.Session.ID, Meta{}); e != nil {
		t.Fatal(e)
	}
	if e = x.redis.Set(ctx, cacheKey, raw, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, token)
	requireCode(t, e, "unauthenticated")
	if _, e = s.Authenticate(ctx, token2); e != nil {
		t.Fatal("other device revoked", e)
	}
	link, e := s.CreateInvitation(ctx, a2, "", "survives voluntary password change", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	oldReset, e := s.ResetLink(ctx, "Alice", Meta{Reason: "身份已核验"})
	if e != nil {
		t.Fatal(e)
	}
	newPassword := "A new 河流 secret 987"
	if e = s.ChangePassword(ctx, a2, testPassword, newPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, token2)
	requireCode(t, e, "unauthenticated")
	if _, e = s.Inspect(ctx, linkToken(link), false, Meta{}); e != nil {
		t.Fatal("voluntary change revoked invitation", e)
	}
	_, e = s.Inspect(ctx, linkToken(oldReset), true, Meta{})
	requireCode(t, e, "invalid_link")
	_, live := login(t, s, "Alice", newPassword)
	reset1, e := s.ResetLink(ctx, "Alice", Meta{Reason: "身份已核验"})
	if e != nil {
		t.Fatal(e)
	}
	reset2, e := s.ResetLink(ctx, "Alice", Meta{Reason: "重发"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(reset1), true, Meta{})
	requireCode(t, e, "invalid_link")
	if _, e = s.Authenticate(ctx, live); e != nil {
		t.Fatal("issuing reset revoked session", e)
	}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() { errs <- s.CompleteReset(ctx, linkToken(reset2), testPassword, Meta{}) })
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			requireCode(t, e, "invalid_link")
		}
	}
	if success != 1 {
		t.Fatal("reset not single use")
	}
	_, e = s.Authenticate(ctx, live)
	requireCode(t, e, "unauthenticated")
	_, e = s.Inspect(ctx, linkToken(link), false, Meta{})
	requireCode(t, e, "invalid_link")
	p, _ := login(t, s, "Alice", testPassword)
	invite(t, s, p, "Bob")
	if e := s.access.RecoverAdmin(ctx, "Bob", "test privileged ownership isolation"); e != nil {
		t.Fatal(e)
	}
	b, btoken := login(t, s, "Bob", testPassword)
	pending, e := s.CreateInvitation(ctx, b, "", "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetStatus(ctx, "Bob", false, false, Meta{Reason: "停用"}); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, btoken)
	requireCode(t, e, "unauthenticated")
	_, e = s.Inspect(ctx, linkToken(pending), false, Meta{})
	requireCode(t, e, "invalid_link")
	disabledReset, e := s.ResetLink(ctx, "Bob", Meta{Reason: "身份已核验"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CompleteReset(ctx, linkToken(disabledReset), newPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
	_, _, _, e = s.Login(ctx, "Bob", newPassword, Meta{})
	requireCode(t, e, "invalid_credentials")
	if e = s.SetStatus(ctx, "Bob", true, false, Meta{Reason: "恢复"}); e != nil {
		t.Fatal(e)
	}
	login(t, s, "Bob", newPassword)
	if e = s.SetStatus(ctx, "Bob", false, false, Meta{Reason: "停用"}); e != nil {
		t.Fatal(e)
	}
	requireCode(t, s.SetStatus(ctx, "Alice", false, false, Meta{Reason: "停用"}), "last_platform_manager")
	if e = s.SetStatus(ctx, "Alice", false, true, Meta{Reason: "确认失陷，应急停用"}); e != nil {
		t.Fatal(e)
	}
	_, e = s.Bootstrap(ctx, "NewUser", "", testPassword, Meta{})
	requireCode(t, e, "already_initialized")
}
func TestIntegrationIsolationExpiryAndFailures(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	a, token := login(t, s, "Alice", testPassword)
	invite(t, s, a, "Bob")
	if e := s.access.RecoverAdmin(ctx, "Bob", "test privileged ownership isolation"); e != nil {
		t.Fatal(e)
	}
	b, btoken := login(t, s, "Bob", testPassword)
	l, e := s.CreateInvitation(ctx, a, "", "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	invitations, e := s.Invitations(ctx, a, "")
	if e != nil {
		t.Fatal(e)
	}
	id := invitations.Items[0].ID
	requireCode(t, s.RevokeInvitation(ctx, b, id, Meta{}), "not_found")
	_, e = s.CreateInvitation(ctx, b, id, "", Meta{})
	requireCode(t, e, "not_found")
	requireCode(t, s.Revoke(ctx, b, a.Session.ID, Meta{}), "not_found")
	regenerated, e := s.CreateInvitation(ctx, a, id, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(l), false, Meta{})
	requireCode(t, e, "invalid_link")
	if _, e = x.pool.Exec(ctx, "UPDATE account_invitations SET expires_at=now()-interval '1 second' WHERE token_hash=$1", digest(linkToken(regenerated))); e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(regenerated), false, Meta{})
	requireCode(t, e, "invalid_link")
	if _, e = x.pool.Exec(ctx, "UPDATE account_sessions SET reauthenticated_at=now()-interval '6 minutes' WHERE id=$1", a.Session.ID); e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateInvitation(ctx, a, "", "", Meta{})
	requireCode(t, e, "reauthentication_required")
	if e = s.Reauthenticate(ctx, a, testPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
	// Reads never extend idle; activity explicitly does, then a conditional flush persists it.
	if _, e = x.pool.Exec(ctx, "UPDATE account_sessions SET last_activity_at=now()-interval '119 minutes' WHERE id=$1", a.Session.ID); e != nil {
		t.Fatal(e)
	}
	a, e = s.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Activity(ctx, a); e != nil {
		t.Fatal(e)
	}
	if e = s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	var at time.Time
	if e = x.pool.QueryRow(ctx, "SELECT last_activity_at FROM account_sessions WHERE id=$1", a.Session.ID).Scan(&at); e != nil || time.Since(at) > time.Minute {
		t.Fatal("activity not durable", e)
	}
	if _, e = x.pool.Exec(ctx, "UPDATE account_sessions SET expires_at=now()-interval '1 second' WHERE id=$1", a.Session.ID); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, token)
	requireCode(t, e, "unauthenticated")
	if _, e = x.pool.Exec(ctx, "UPDATE account_sessions SET last_activity_at=now()-interval '3 hours' WHERE id=$1", b.Session.ID); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, btoken)
	requireCode(t, e, "unauthenticated")
	for i := 0; i < 5; i++ {
		_, _, _, e = s.Login(ctx, "Unknown", "wrong password", Meta{})
		requireCode(t, e, "invalid_credentials")
	}
	_, _, _, e = s.Login(ctx, "unknown", testPassword, Meta{})
	requireCode(t, e, "rate_limited")
	// Persistent cooldown survives cache eviction and service recreation.
	var failures int
	if e = x.pool.QueryRow(ctx, "SELECT failures FROM account_rate_limits WHERE key_hash=$1", digest("password:unknown")).Scan(&failures); e != nil || failures != 5 {
		t.Fatal("cooldown not persisted", e)
	}
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, MaxRetries: 0})
	defer dead.Close()
	original := s.cache
	s.cache = NewCache(dead, x.cfg.RedisKeyPrefix, slog.Default())
	_, e = s.Authenticate(ctx, btoken)
	if e == nil || errors.Is(e, ErrUnauthorized) {
		t.Fatal("Redis failure treated as miss")
	}
	if e = s.SetStatus(ctx, "Bob", false, false, Meta{Reason: "缓存故障维护"}); e != nil {
		t.Fatal(e)
	}
	if e = s.DrainInvalidations(ctx); e == nil {
		t.Fatal("failed Redis cleanup not reported")
	}
	s.cache = original
	if e = s.DrainInvalidations(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.RestoreRevoke(ctx, Meta{Reason: "备份恢复"}); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = x.pool.QueryRow(ctx, "SELECT count(*) FROM account_sessions WHERE revoked_at IS NULL").Scan(&count); e != nil || count != 0 {
		t.Fatal("restore did not revoke all sessions", e)
	}
	// Durable credential storage never contains raw bearer tokens.
	var hash string
	if e = x.pool.QueryRow(ctx, "SELECT token_hash FROM account_sessions WHERE id=$1", a.Session.ID).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	if _, e = hex.DecodeString(hash); e != nil || len(hash) != 64 || hash == token {
		t.Fatal("session not hashed")
	}
}

func TestIntegrationPasswordBoundariesAtEveryEntry(t *testing.T) {
	ctx := context.Background()
	for _, length := range []int{8, 128} {
		t.Run("bootstrap_"+strconv.Itoa(length), func(t *testing.T) {
			x := setup(t)
			s := x.service
			for _, bad := range []int{7, 129} {
				_, e := s.Bootstrap(ctx, "Alice", "", strings.Repeat("中🙂", bad/2)+strings.Repeat("X", bad%2), Meta{})
				requireCode(t, e, "validation_failed")
				initialized, e := s.Initialized(ctx)
				if e != nil || initialized {
					t.Fatal("invalid bootstrap marked initialized", e)
				}
			}
			secret := strings.Repeat("中🙂", length/2)
			if _, e := s.Bootstrap(ctx, "Alice", "", secret, Meta{}); e != nil {
				t.Fatal(e)
			}
			login(t, s, "Alice", secret)
		})
	}
	x := setup(t)
	s := x.service
	bootstrap(t, s)
	current := testPassword
	for _, length := range []int{7, 8, 128, 129} {
		secret := strings.Repeat("中🙂", length/2) + strings.Repeat("X", length%2)
		valid := length == 8 || length == 128
		p, _ := login(t, s, "Alice", current)
		l, e := s.CreateInvitation(ctx, p, "", "", Meta{})
		if e != nil {
			t.Fatal(e)
		}
		e = s.AcceptInvitation(ctx, linkToken(l), "user_"+strconv.Itoa(length), "", secret, Meta{})
		if valid {
			if e != nil {
				t.Fatal(e)
			}
			login(t, s, "user_"+strconv.Itoa(length), secret)
		} else {
			requireCode(t, e, "validation_failed")
			if _, e = s.Inspect(ctx, linkToken(l), false, Meta{}); e != nil {
				t.Fatal("invalid signup consumed invitation")
			}
		}
		e = s.ChangePassword(ctx, p, current, secret, Meta{})
		if valid {
			if e != nil {
				t.Fatal(e)
			}
			current = secret
		} else {
			requireCode(t, e, "validation_failed")
		}
		reset, e := s.ResetLink(ctx, "Alice", Meta{Reason: "boundary test"})
		if e != nil {
			t.Fatal(e)
		}
		e = s.CompleteReset(ctx, linkToken(reset), secret, Meta{})
		if valid {
			if e != nil {
				t.Fatal(e)
			}
			current = secret
		} else {
			requireCode(t, e, "validation_failed")
			if _, e = s.Inspect(ctx, linkToken(reset), true, Meta{}); e != nil {
				t.Fatal("invalid reset consumed token")
			}
		}
	}
}

func TestIntegrationLimitsAuditCleanupAndRollback(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, token := login(t, s, "Alice", testPassword)
	s.cfg.InvitationLimit = 1
	link, e := s.CreateInvitation(ctx, p, "", "only one", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateInvitation(ctx, p, "", "over limit", Meta{})
	requireCode(t, e, "invitation_limit")
	items, e := s.Invitations(ctx, p, "")
	if e != nil {
		t.Fatal(e)
	}
	id := items.Items[0].ID
	replacement, e := s.CreateInvitation(ctx, p, id, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(link), false, Meta{})
	requireCode(t, e, "invalid_link")
	// Revoked invitations are retained with their original history.
	items, e = s.Invitations(ctx, p, "")
	if e != nil || len(items.Items) != 2 {
		t.Fatal("regeneration overwrote history", e)
	}
	if e = s.RevokeInvitation(ctx, p, items.Items[0].ID, Meta{}); e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(replacement), false, Meta{})
	requireCode(t, e, "invalid_link")
	s.cfg.InvitationRate = 1 // use a fresh rate namespace without removing other test counters.
	s.cache.prefix = x.cfg.RedisKeyPrefix + "rate-check:"
	if _, e = s.CreateInvitation(ctx, p, "", "rate", Meta{}); e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateInvitation(ctx, p, "", "rate", Meta{})
	requireCode(t, e, "rate_limited")
	s.cache.prefix = x.cfg.RedisKeyPrefix
	// Failed passwords persist across service restart, but cooldown expiry permits correct login.
	for i := 0; i < 5; i++ {
		_, _, _, e = s.Login(ctx, "Alice", "wrong secret", Meta{})
		requireCode(t, e, "invalid_credentials")
	}
	_, _, _, e = s.Login(ctx, "Alice", testPassword, Meta{})
	requireCode(t, e, "rate_limited")
	if _, e = x.pool.Exec(ctx, "UPDATE account_rate_limits SET cooldown_until=now()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	login(t, s, "Alice", testPassword)
	// Reads leave activity unchanged; a delayed flush cannot resurrect a revoked session.
	old := p.Session.LastActivityAt
	for i := 0; i < 3; i++ {
		p, e = s.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
	}
	if !old.Equal(p.Session.LastActivityAt) {
		t.Fatal("reads extended idle")
	}
	if e = s.Activity(ctx, p); e != nil {
		t.Fatal(e)
	}
	if e = s.repo.Revoke(ctx, p, p.Session.ID, s.cfg.SessionIdle, Meta{}); e != nil {
		t.Fatal(e)
	}
	if e = s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	_, e = s.Authenticate(ctx, token)
	requireCode(t, e, "unauthenticated")
	// Events are cursor-paginated and tied to the owner; cleanup retains initialization.
	live, _ := login(t, s, "Alice", testPassword)
	for i := 0; i < 55; i++ {
		if _, e = s.Profile(ctx, live, "名称", Meta{}); e != nil {
			t.Fatal(e)
		}
	}
	page, e := s.SecurityEvents(ctx, live, "")
	if e != nil || len(page.Items) != 50 || page.NextCursor == "" {
		t.Fatal("missing pagination", e)
	}
	more, e := s.SecurityEvents(ctx, live, page.NextCursor)
	if e != nil || len(more.Items) == 0 || more.Items[0].ID >= page.Items[49].ID {
		t.Fatal("cursor duplicate or gap", e)
	}
	if _, e = x.pool.Exec(ctx, "UPDATE account_security_events SET created_at=now()-interval '100 days'; UPDATE account_invitations SET expires_at=now()-interval '100 days'"); e != nil {
		t.Fatal(e)
	}
	if e = s.repo.Cleanup(ctx, s.cfg.AuditRetention); e != nil {
		t.Fatal(e)
	}
	initialized, e := s.Initialized(ctx)
	if e != nil || !initialized {
		t.Fatal("cleanup cleared initialization")
	}
}

func TestIntegrationInitializationRollbackAndStaleCacheAfterMutation(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	_, e := x.pool.Exec(ctx, `CREATE FUNCTION fail_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected audit failure'; END $$; CREATE TRIGGER fail_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION fail_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Bootstrap(ctx, "Alice", "", testPassword, Meta{}); e == nil {
		t.Fatal("injected failure did not roll back")
	}
	initialized, e := s.Initialized(ctx)
	if e != nil || initialized {
		t.Fatal("partial bootstrap committed")
	}
	var count int
	if e = x.pool.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&count); e != nil || count != 0 {
		t.Fatal("account survived rolled back bootstrap")
	}
	if _, e = x.pool.Exec(ctx, "DROP TRIGGER fail_audit ON account_security_events"); e != nil {
		t.Fatal(e)
	}
	bootstrap(t, s)
	for _, action := range []string{"change", "reset", "disable"} {
		t.Run(action, func(t *testing.T) {
			p, token := login(t, s, "Alice", testPassword)
			key := x.cfg.RedisKeyPrefix + "session:" + digest(token)
			raw, e := x.redis.Get(ctx, key).Result()
			if e != nil {
				t.Fatal(e)
			}
			switch action {
			case "change":
				if e = s.ChangePassword(ctx, p, testPassword, "Updated secret 河流 246", Meta{}); e != nil {
					t.Fatal(e)
				}
			case "reset":
				link, e := s.ResetLink(ctx, "Alice", Meta{Reason: "verified test"})
				if e != nil {
					t.Fatal(e)
				}
				if e = s.CompleteReset(ctx, linkToken(link), testPassword, Meta{}); e != nil {
					t.Fatal(e)
				}
			case "disable":
				if e = s.SetStatus(ctx, "Alice", false, true, Meta{Reason: "emergency test"}); e != nil {
					t.Fatal(e)
				}
			}
			if e = x.redis.Set(ctx, key, raw, time.Minute).Err(); e != nil {
				t.Fatal(e)
			}
			_, e = s.Authenticate(ctx, token)
			requireCode(t, e, "unauthenticated")
			if action == "change" {
				link, e := s.ResetLink(ctx, "Alice", Meta{Reason: "return test credential"})
				if e != nil {
					t.Fatal(e)
				}
				if e = s.CompleteReset(ctx, linkToken(link), testPassword, Meta{}); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestIntegrationExpiredResetAndLogoutAll(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	a, token := login(t, s, "Alice", testPassword)
	_, other := login(t, s, "Alice", testPassword)
	link, e := s.ResetLink(ctx, "Alice", Meta{Reason: "expiry test"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = x.pool.Exec(ctx, "UPDATE account_password_resets SET expires_at=now()-interval '1 second'"); e != nil {
		t.Fatal(e)
	}
	_, e = s.Inspect(ctx, linkToken(link), true, Meta{})
	requireCode(t, e, "invalid_link")
	requireCode(t, s.CompleteReset(ctx, linkToken(link), "Expired secret must not apply", Meta{}), "invalid_link")
	if _, e = s.Authenticate(ctx, token); e != nil {
		t.Fatal("expired reset affected existing login", e)
	}
	if e = s.Revoke(ctx, a, "", Meta{}); e != nil {
		t.Fatal(e)
	}
	for _, cookie := range []string{token, other} {
		_, e = s.Authenticate(ctx, cookie)
		requireCode(t, e, "unauthenticated")
	}
	if e = s.Logout(ctx, token, Meta{}); e != nil {
		t.Fatal("expired logout should be idempotent", e)
	}
}
