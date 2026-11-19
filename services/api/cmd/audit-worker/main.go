package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/auxworker"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := run(); e != nil {
		slog.Error("audit worker stopped", "code", "audit_worker_unavailable")
		os.Exit(1)
	}
}
func run() (resultErr error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if resultErr != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, opts, e := config.LoadAudit(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	slot, e := auxworker.Pool(ctx, "audit-worker", db.URL, 0)
	if e != nil {
		return e
	}
	defer slot.CloseResult(&resultErr)
	pool, ctx := slot.Pool, slot.Context
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	s := runtime.Compose(access.New(pool, slog.Default())).Audit.WithOptions(opts)
	if opts.ObjectEndpoint != "" {
		budget, e := config.WorkerObjectBudget("audit-worker")
		if e != nil {
			return e
		}
		if budget != nil {
			diagnosticreport.SetMetric(reporter, "quota.s3_inflight", int64(budget.Capacity()))
		}
		store, e := blobstore.Open(opts.ObjectEndpoint, opts.ObjectAccessKey, opts.ObjectSecretKey, opts.Bucket, "audit-exports", budget)
		if e != nil {
			return e
		}
		s.WithStorage(store)
	}
	reporter.Ready()
	slog.Info("audit worker ready")
	return auxworker.Run(ctx, pool, "audit", s.RunExportGeneration, func(ctx context.Context) error { return s.RunWithFallback(ctx, eventbus.FallbackInterval(), reporter) }, reporter)
}
