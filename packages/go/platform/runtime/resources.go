package runtime

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"log/slog"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Resources struct {
	Pool     *pgxpool.Pool
	Redis    *redis.Client
	Identity *identity.Service
}

func Open(ctx context.Context, cfg config.Config, logger *slog.Logger, requireRedis bool) (*Resources, error) {
	pc, e := pgxpool.ParseConfig(cfg.DatabaseURL)
	if e != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	pc.MaxConns = 12
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return nil, fmt.Errorf("database connection unavailable")
	}
	fail := func(err error) (*Resources, error) { pool.Close(); return nil, err }
	if e = pool.Ping(ctx); e != nil {
		return fail(fmt.Errorf("database unavailable"))
	}
	if e = migrations.Check(ctx, pool); e != nil {
		return fail(e)
	}
	if e = access.New(pool, logger).Check(ctx); e != nil {
		return fail(e)
	}
	ro, e := redis.ParseURL(cfg.RedisURL)
	if e != nil {
		return fail(fmt.Errorf("invalid Redis configuration"))
	}
	ro.DialTimeout = 2 * time.Second
	ro.ReadTimeout = 2 * time.Second
	ro.WriteTimeout = 2 * time.Second
	ro.MaxRetries = 1
	ro.ContextTimeoutEnabled = true
	client := redis.NewClient(ro)
	if requireRedis {
		if e = client.Ping(ctx).Err(); e != nil {
			client.Close()
			return fail(fmt.Errorf("Redis unavailable"))
		}
	}
	service, e := identity.NewService(pool, client, cfg, logger)
	if e != nil {
		client.Close()
		return fail(e)
	}
	return &Resources{Pool: pool, Redis: client, Identity: service}, nil
}
func (r *Resources) Close() { _ = r.Redis.Close(); r.Pool.Close() }
