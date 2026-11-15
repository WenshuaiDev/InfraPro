package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
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
		slog.Error("task index worker stopped", "error", e)
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
	pc, e := pgxpool.ParseConfig(db.URL)
	if e != nil {
		return e
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	pc.MaxConns = 4
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	index := runtime.ComposeTaskIndexer(access.New(pool, slog.Default())).WithOptions(tasks.Options(o))
	reporter.Ready()
	slog.Info("task index worker ready")
	index.Run(ctx, uuid.NewString(), func(error) { slog.Warn("task index iteration failed; inspect tasks status") }, reporter)
	return nil
}
