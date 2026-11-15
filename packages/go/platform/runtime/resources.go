package runtime

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"log/slog"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Resources struct {
	Pool       *pgxpool.Pool
	Redis      *redis.Client
	Identity   *identity.Service
	Monitoring *monitoring.Storage
}

func Open(ctx context.Context, cfg config.Config, logger *slog.Logger, requireRedis bool) (*Resources, error) {
	return open(ctx, cfg, logger, requireRedis, 12)
}
func OpenDisplay(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Resources, error) {
	return open(ctx, cfg, logger, false, int32(cfg.Display.PoolSize))
}
func open(ctx context.Context, cfg config.Config, logger *slog.Logger, requireRedis bool, poolSize int32) (*Resources, error) {
	pc, e := pgxpool.ParseConfig(cfg.DatabaseURL)
	if e != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	pc.MaxConns = poolSize
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
	var storage *monitoring.Storage
	if len(cfg.Monitoring.Replicas) > 0 {
		storage, e = monitoring.OpenStorage(ctx, MonitoringStorageConfig(cfg.Monitoring))
		if e != nil {
			client.Close()
			return fail(e)
		}
		if e = storage.Check(ctx); e != nil {
			storage.Close()
			client.Close()
			return fail(fmt.Errorf("monitoring storage not ready; run infrapro monitoring storage check"))
		}
		if e = derivedmonitoring.CheckStorage(ctx, storage); e != nil {
			storage.Close()
			client.Close()
			return fail(fmt.Errorf("derived result storage not ready; run infrapro monitoring storage init: %w", e))
		}
	}
	return &Resources{Pool: pool, Redis: client, Identity: service, Monitoring: storage}, nil
}
func (r *Resources) Close() { r.Monitoring.Close(); _ = r.Redis.Close(); r.Pool.Close() }
