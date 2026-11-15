package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pc, e := pgxpool.ParseConfig(db.URL)
	if e != nil {
		return e
	}
	pc.MaxConns = 2
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = pool.Ping(ctx); e != nil {
		return e
	}
	objects, e := blobstore.Open(storage.ObjectEndpoint, storage.ObjectAccessKey, storage.ObjectSecretKey, storage.Bucket, "display")
	if e != nil {
		return e
	}
	retention, e := config.LoadDisplayRetention(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	options := display.DefaultOptions()
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
