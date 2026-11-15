package main

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("spatial worker stopped", "error", e)
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
	cfg, e := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	converter := spatial.PythonConverter{Python: cfg.SpatialPython, Script: cfg.SpatialConverterScript}
	reporter.Begin("converter", diagnosticreport.Deadline(ctx))
	if e = converter.Ready(diagnosticreport.ObserveDeadlines(ctx, reporter, "converter")); e != nil {
		reporter.End("converter", "configuration_invalid", time.Time{})
		return e
	}
	reporter.End("converter", "", time.Time{})
	reporter.Dependency("converter", true, "")
	host, e := runtime.Open(ctx, cfg, logger, false)
	if e != nil {
		return e
	}
	defer host.Close()
	business := runtime.Compose(host.Identity.Access(), host.Monitoring).Configure(cfg)
	reporter.Ready()
	logger.Info("spatial worker ready")
	business.Spatial.RunWorker(ctx, uuid.NewString(), func(e error) { logger.Warn("spatial task did not complete", "error", e) }, reporter)
	return nil
}
