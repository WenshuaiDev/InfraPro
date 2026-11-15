package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("notification worker stopped", "error", e)
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
	cfg, options, e := config.LoadNotifications(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pc, e := pgxpool.ParseConfig(cfg.URL)
	if e != nil {
		return e
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	pc.MaxConns = 6
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	a := access.New(pool, slog.Default())
	if e = a.Check(ctx); e != nil {
		return e
	}
	s, e := runtime.ComposeNotifications(a).WithOptions(notifications.Options(options))
	if e != nil {
		return e
	}
	if e = s.Check(ctx); e != nil {
		return e
	}
	reporter.Ready()
	slog.Info("notification worker ready")
	s.Run(ctx, uuid.NewString(), func(e error) { slog.Warn("notification worker iteration failed", "error", e) }, reporter)
	return nil
}
