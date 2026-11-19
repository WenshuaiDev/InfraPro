package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/auxworker"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := run(); e != nil {
		slog.Error("notification worker stopped", "code", "delivery_unavailable")
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
	slot, e := auxworker.Pool(ctx, "notification-worker", cfg.URL, 6)
	if e != nil {
		return e
	}
	defer slot.CloseResult(&resultErr)
	pool, ctx := slot.Pool, slot.Context
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
	return auxworker.Run(ctx, pool, "notification", s.DeliverEvent, func(ctx context.Context) error {
		s.RunWithFallback(ctx, eventbus.FallbackInterval(), uuid.NewString(), func(error) { slog.Warn("notification worker iteration failed", "code", "delivery_unavailable") }, reporter)
		return nil
	}, reporter)
}
