package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/auxworker"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("display package worker stopped", "error", e)
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
	db, storage, e := config.LoadDisplayWorker(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slot, e := auxworker.Pool(ctx, "display-package-worker", db.URL, 2)
	if e != nil {
		return e
	}
	defer slot.CloseResult(&resultErr)
	pool, ctx := slot.Pool, slot.Context
	diagnosticreport.SetMetric(reporter, "quota.pg_connections", int64(pool.Config().MaxConns))
	if e = pool.Ping(ctx); e != nil {
		return e
	}
	budget, e := config.WorkerObjectBudget("display-package-worker")
	if e != nil {
		return e
	}
	if budget != nil {
		diagnosticreport.SetMetric(reporter, "quota.s3_inflight", int64(budget.Capacity()))
	}
	objects, e := blobstore.Open(storage.ObjectEndpoint, storage.ObjectAccessKey, storage.ObjectSecretKey, storage.Bucket, "display", budget)
	if e != nil {
		return e
	}
	retention, e := config.LoadDisplayRetention(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	limits, e := config.LoadUploads(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	options := display.DefaultOptions()
	options.MaxPackageBytes = limits.DisplayMaxPackageBytes
	options.EventRetention = retention
	s := display.New(access.New(pool, slog.Default())).WithOptions(options).WithStorage(objects)
	reporter.Ready()
	slog.Info("display package worker ready")
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				slog.Info("display package worker metrics", "metrics", s.Metrics())
			}
		}
	}()
	s.RunWorker(ctx, func(e error) { slog.Warn("display package processing failed", "code", "package_processing_failed") }, reporter)
	return nil
}
