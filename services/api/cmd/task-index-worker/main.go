package main

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/auxworker"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := run(); e != nil {
		slog.Error("task index worker stopped", "code", "index_unavailable")
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
	db, o, e := config.LoadTasks(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	slot, e := auxworker.Pool(ctx, "task-index-worker", db.URL, 4)
	if e != nil {
		return e
	}
	defer slot.CloseResult(&resultErr)
	pool, ctx := slot.Pool, slot.Context
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	index := runtime.ComposeTaskIndexer(access.New(pool, slog.Default())).WithOptions(tasks.Options(o))
	reporter.Ready()
	slog.Info("task index worker ready")
	return auxworker.Run(ctx, pool, "task-index", func(ctx context.Context, id string, rev int64, scope string) (bool, error) {
		err := index.ProjectEvent(ctx, id, rev, scope)
		if errors.Is(err, tasks.ErrProjectionPaused) {
			return true, nil
		}
		return false, err
	}, func(ctx context.Context) error {
		index.RunWithFallback(ctx, eventbus.FallbackInterval(), uuid.NewString(), func(error) { slog.Warn("task index iteration failed; inspect tasks status") }, reporter)
		return nil
	}, reporter)
}
