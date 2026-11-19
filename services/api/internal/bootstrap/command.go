package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
)

func APIMain(role string) {
	if err := runAPI(role); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func runAPI(role string) (resultErr error) {
	if role == config.Gateway && len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "routes" {
			return fmt.Errorf("usage: api [routes]")
		}
		tables := map[string][]routes.Operation{}
		for _, owner := range append([]string{"", config.Gateway}, config.Owners...) {
			key := owner
			if key == "" {
				key = "compat-api"
			}
			tables[key] = httptransport.RegisteredOperations(owner)
		}
		return json.NewEncoder(os.Stdout).Encode(tables)
	}
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if resultErr != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	cfg, err := config.LoadRole(os.Getenv("INFRAPRO_CONFIG"), role)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, cfg, logger, reporter)
}
