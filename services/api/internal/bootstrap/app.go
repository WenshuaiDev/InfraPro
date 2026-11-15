package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger, reporters ...diagnosticreport.Sink) error {
	reporter := diagnosticreport.Optional(reporters)
	resources, err := runtime.Open(ctx, cfg, logger, true)
	reporter.Dependency("bootstrap", err == nil, diagnosticreport.Result(err))
	if err != nil {
		return err
	}
	defer resources.Close()
	jobs, cancelJobs := context.WithCancel(ctx)
	jobsDone := make(chan struct{})
	go func() { defer close(jobsDone); resources.Identity.RunJobs(jobs) }()
	defer func() {
		cancelJobs()
		<-jobsDone
		flush, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if resources.Identity.Flush(flush) != nil {
			logger.Warn("final activity flush incomplete")
		}
	}()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	server := &http.Server{
		Handler:           diagnosticreport.Readiness(httptransport.NewRouter(logger, resources.Identity, cfg, resources.Monitoring), reporter),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	reporter.Ready()
	reporter.Idle("http", time.Time{})
	logger.Info("api listening", "address", listener.Addr().String())
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown api: %w", err)
		}
		logger.Info("api stopped gracefully")
		return nil
	}
}
