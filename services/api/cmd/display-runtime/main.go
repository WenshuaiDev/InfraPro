package main

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	displayhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/display"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		slog.Error("display runtime stopped", "error", e)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	host, e := runtime.OpenDisplay(ctx, cfg, logger)
	if e != nil {
		return e
	}
	defer host.Close()
	business := runtime.Compose(host.Access, host.Monitoring).Configure(cfg)
	servers := []*http.Server{}
	listeners := []net.Listener{}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for addr, handler := range map[string]http.Handler{cfg.Display.RuntimeAddr: displayhttp.RuntimeHandler(business.Displays), cfg.Display.AssetAddr: displayhttp.AssetHandler(business.Displays)} {
		l, e := net.Listen("tcp", addr)
		if e != nil {
			return e
		}
		listeners = append(listeners, l)
		lane := "runtime-http"
		if addr == cfg.Display.AssetAddr {
			lane = "assets-http"
		}
		reporter.Idle(lane, time.Time{})
		servers = append(servers, &http.Server{Handler: diagnosticreport.Readiness(handler, reporter), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10})
	}
	done := make(chan error, 2)
	for i, s := range servers {
		go func() { done <- s.Serve(listeners[i]) }()
	}
	reporter.Ready()
	logger.Info("display runtime and isolated asset endpoints ready")
	select {
	case e = <-done:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(shutdown); err != nil {
			_ = s.Close()
		}
	}
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
