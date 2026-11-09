package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/config"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}
	server := &http.Server{
		Handler:           httptransport.NewRouter(logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
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
