package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if run() != nil {
		slog.Error("ingest worker stopped", "code", "dependency_unavailable")
		os.Exit(1)
	}
}
func run() (result error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if result != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	cfg, e := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	resources, e := runtime.OpenWorker(ctx, cfg, logger)
	if e != nil {
		return e
	}
	defer resources.Close()
	b := runtime.Compose(resources.Access, resources.Monitoring).Configure(cfg)
	if e = b.Ingestion.Ready(ctx); e != nil {
		return e
	}
	reporter.Ready()
	return b.Ingestion.RunWorker(ctx, reporter)
}
