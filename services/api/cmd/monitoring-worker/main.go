package main

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func main() {
	if e := run(); e != nil {
		slog.Error("monitoring worker stopped", "error", e)
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
	if len(cfg.Monitoring.Replicas) == 0 && !cfg.Parsing.Enabled {
		return fmt.Errorf("monitoring storage is not configured")
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	resources, e := runtime.Open(ctx, cfg, logger, false)
	if e != nil {
		return e
	}
	defer resources.Close()
	business := runtime.Compose(resources.Identity.Access(), resources.Monitoring).Configure(cfg)
	if len(cfg.Monitoring.Replicas) > 0 {
		if e = business.DerivedMonitoring.Ready(ctx); e != nil {
			return e
		}
	}
	reporter.Ready()
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _ = business.Parsing.RunWorker(work, reporter) }()
	var err error
	if len(cfg.Monitoring.Replicas) > 0 {
		err = business.Monitoring.RunWorker(work, logger, reporter)
	} else {
		<-work.Done()
	}
	cancel()
	<-done
	return err
}
