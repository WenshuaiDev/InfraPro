// Package auxworker runs the ordinary clock-driven maintenance alongside
// durable keyed delivery. Shutdown waits for both before the pool is closed.
package auxworker

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// Pool keeps each process within its configured budget. Zero keeps pgx's
// compatible default when the deployment specifies no role budget.
func Pool(ctx context.Context, role, url string, fallback int32) (*replicabudget.Pool, error) {
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if fallback == 0 {
		fallback = pc.MaxConns
	}
	arch, err := config.WorkerArchitecture(role)
	if err != nil {
		return nil, err
	}
	pc.MaxConns = fallback
	if arch != nil {
		if arch.Pools[role] > 0 {
			pc.MaxConns = arch.Connections(role)
		} else {
			pc.MaxConns = int32(arch.Share(int(fallback)))
		}
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	return replicabudget.Open(ctx, pc, arch.SlotKey())
}

type Work func(context.Context, string, int64, string) (bool, error)

func Run(ctx context.Context, pool *pgxpool.Pool, name string, work Work, scan func(context.Context) error, reporter diagnosticreport.Sink) error {
	diagnosticreport.SetMetric(reporter, "quota.pg_connections", int64(pool.Config().MaxConns))
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return eventbus.RunConsumer(ctx, pool, name, func(ctx context.Context, event eventbus.Event) error {
			pending, err := work(ctx, event.AggregateID, event.Generation, event.Scope)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				return eventbus.ErrBadReference
			case err != nil:
				return err
			case pending:
				return eventbus.ErrDeferred
			default:
				return nil
			}
		}, reporter)
	})
	group.Go(func() error { return scan(ctx) })
	return group.Wait()
}
