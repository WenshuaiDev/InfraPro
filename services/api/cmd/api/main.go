package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/bootstrap"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return bootstrap.Run(ctx, cfg, logger)
}
