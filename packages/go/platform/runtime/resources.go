package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"log/slog"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Resources struct {
	slot       *replicabudget.Pool
	Context    context.Context
	Pool       *pgxpool.Pool
	Redis      *redis.Client
	Identity   *identity.Service
	Access     *access.Service
	Monitoring *monitoring.Storage
}

func Open(ctx context.Context, cfg config.Config, logger *slog.Logger, requireRedis bool) (*Resources, error) {
	return open(ctx, cfg, logger, requireRedis, 12, true)
}

// OpenWorker omits personnel authentication and its decryption keys. Workers
// still share the database/catalog checks and transaction authorization rules.
func OpenWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Resources, error) {
	return open(ctx, cfg, logger, false, 12, false)
}
func OpenDisplay(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Resources, error) {
	return open(ctx, cfg, logger, false, int32(cfg.Display.PoolSize), false)
}
func open(ctx context.Context, cfg config.Config, logger *slog.Logger, requireRedis bool, poolSize int32, personnelIdentity bool) (*Resources, error) {
	role := ""
	if cfg.Architecture != nil {
		role = cfg.Architecture.Role
		poolSize = cfg.Architecture.Connections(role)
	}
	pc, e := pgxpool.ParseConfig(cfg.DatabaseURL)
	if e != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	pc.MaxConns = poolSize
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	slot, e := replicabudget.Open(ctx, pc, cfg.Architecture.SlotKey())
	if e != nil {
		if errors.Is(e, replicabudget.ErrOccupied) {
			return nil, replicabudget.ErrOccupied
		}
		return nil, fmt.Errorf("database connection unavailable")
	}
	pool, ctx := slot.Pool, slot.Context
	fail := func(err error) (*Resources, error) { slot.Close(); return nil, err }
	if e = pool.Ping(ctx); e != nil {
		return fail(fmt.Errorf("database unavailable"))
	}
	if e = migrations.Check(ctx, pool); e != nil {
		return fail(e)
	}
	authorization := access.New(pool, logger)
	if e = authorization.Check(ctx); e != nil {
		return fail(e)
	}
	var client *redis.Client
	if cfg.RedisURL != "" {
		ro, e := redis.ParseURL(cfg.RedisURL)
		if e != nil {
			return fail(fmt.Errorf("invalid Redis configuration"))
		}
		ro.DialTimeout = 2 * time.Second
		ro.ReadTimeout = 2 * time.Second
		ro.WriteTimeout = 2 * time.Second
		ro.MaxRetries = 1
		ro.ContextTimeoutEnabled = true
		client = redis.NewClient(ro)
		if requireRedis {
			if e = client.Ping(ctx).Err(); e != nil {
				closeRedis(client)
				return fail(fmt.Errorf("Redis unavailable"))
			}
		}
	}
	var service *identity.Service
	if personnelIdentity || role == config.Control || role == config.Query || role == config.Transfer {
		newService := identity.NewDomainService
		if personnelIdentity {
			newService = identity.NewService
		}
		service, e = newService(pool, client, cfg, logger)
		if e != nil {
			closeRedis(client)
			return fail(e)
		}
		authorization = service.Access()
	}
	var storage *monitoring.Storage
	useStorage := role == "" || role == "compat-api" || role == config.Control || role == config.Query || role == config.Transfer || strings.HasPrefix(role, "monitoring-") || role == "display-runtime" && !cfg.Architecture.DisplaySplit
	if useStorage && len(cfg.Monitoring.Replicas) > 0 {
		openStorage := monitoring.OpenStorage
		lazy := role == config.Control || role == config.Query || role == config.Transfer
		if lazy {
			openStorage = monitoring.ConnectStorage
		}
		sc := MonitoringStorageConfig(cfg.Monitoring)
		if cfg.Architecture != nil {
			sc.ObjectRequests = cfg.Architecture.ObjectRequests()
			endpoints := len(sc.Replicas)
			for _, shard := range sc.Shards {
				endpoints += len(shard.Replicas)
			}
			if endpoints < 1 || cfg.Architecture.Inflight(role) < endpoints {
				closeRedis(client)
				return fail(fmt.Errorf("ClickHouse role quota below physical replica count"))
			}
			sc.ConnectionsPerReplica = min(6, cfg.Architecture.Inflight(role)/endpoints)
		}
		storage, e = openStorage(ctx, sc)
		if e != nil {
			closeRedis(client)
			return fail(e)
		}
		if !lazy {
			if e = storage.Check(ctx); e != nil {
				storage.Close()
				closeRedis(client)
				return fail(fmt.Errorf("monitoring storage not ready; run infrapro monitoring storage check"))
			}
			if e = derivedmonitoring.CheckStorage(ctx, storage); e != nil {
				storage.Close()
				closeRedis(client)
				return fail(fmt.Errorf("derived result storage not ready; run infrapro monitoring storage init: %w", e))
			}
		}
	}
	return &Resources{slot: slot, Context: ctx, Pool: pool, Redis: client, Identity: service, Access: authorization, Monitoring: storage}, nil
}
func (r *Resources) Close() { r.Monitoring.Close(); closeRedis(r.Redis); r.slot.Close() }
func (r *Resources) CloseResult(result *error) {
	if errors.Is(context.Cause(r.Context), replicabudget.ErrLost) {
		*result = replicabudget.ErrLost
	}
	r.Close()
}

func closeRedis(client *redis.Client) {
	if client != nil {
		_ = client.Close()
	}
}
