// Package testsupport provisions isolated schemas only inside explicitly named *_test databases.
// It is imported exclusively by tests; production startup never seeds accounts.
package testsupport

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Environment struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Redis   *redis.Client
	Service *identity.Service
	cleanup func()
}

func (e *Environment) Close() { e.cleanup() }
func New(ctx context.Context, logger *slog.Logger) (*Environment, error) {
	dbURL := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	redisURL := os.Getenv("INFRAPRO_TEST_REDIS_URL")
	u, e := url.Parse(dbURL)
	if e != nil || u.Host == "" || !strings.HasSuffix(u.Path, "_test") || redisURL == "" {
		return nil, fmt.Errorf("explicit test database ending in _test and test Redis URL required")
	}
	admin, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		return nil, e
	}
	schema := "identity_test_" + strings.ToLower(rand.Text())
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		admin.Close()
		return nil, e
	}
	cleanupDB := func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); admin.Close() }
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dbURL = u.String()
	if e = migrations.Up(ctx, dbURL); e != nil {
		cleanupDB()
		return nil, e
	}
	pool, e := pgxpool.New(ctx, dbURL)
	if e != nil {
		cleanupDB()
		return nil, e
	}
	options, e := redis.ParseURL(redisURL)
	if e != nil {
		pool.Close()
		cleanupDB()
		return nil, e
	}
	options.MaxRetries = 0
	options.DialTimeout = time.Second
	options.ReadTimeout = time.Second
	options.WriteTimeout = time.Second
	cache := redis.NewClient(options)
	cfg := config.Config{DatabaseURL: dbURL, RedisURL: redisURL, RedisKeyPrefix: "infrapro:test-" + schema + ":identity:v1:", PublicConsoleURL: "http://127.0.0.1:15173/", TrustedOrigins: []string{"http://127.0.0.1:15173"}, SupportContact: "测试维护人员：内线 8000", CSRFKey: []byte("0123456789abcdef0123456789abcdef"), SessionIdle: 2 * time.Hour, SessionAbsolute: 12 * time.Hour, SessionCacheTTL: 5 * time.Minute, InvitationTTL: 24 * time.Hour, ResetTTL: 30 * time.Minute, AuditRetention: 90 * 24 * time.Hour, InvitationRate: 1000, InvitationLimit: 20, IPRequestLimit: 10000, CredentialRequestLimit: 1000, PasswordFailures: 5}
	if e = access.New(pool, logger).Sync(ctx); e != nil {
		cache.Close()
		pool.Close()
		cleanupDB()
		return nil, e
	}
	service, e := identity.NewService(pool, cache, cfg, logger)
	if e != nil {
		cache.Close()
		pool.Close()
		cleanupDB()
		return nil, e
	}
	return &Environment{Config: cfg, Pool: pool, Redis: cache, Service: service, cleanup: func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var cursor uint64
		for {
			keys, next, err := cache.Scan(ctx, cursor, cfg.RedisKeyPrefix+"*", 100).Result()
			if err != nil {
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
		cache.Close()
		pool.Close()
		cleanupDB()
	}}, nil
}
