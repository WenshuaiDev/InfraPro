package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	displayhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/display"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func DisplayMain(role string) {
	if e := runDisplay(role); e != nil {
		slog.Error("display runtime stopped", "error", e)
		os.Exit(1)
	}
}
func runDisplay(role string) (resultErr error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if resultErr != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	cfg, e := config.LoadRole(os.Getenv("INFRAPRO_CONFIG"), role)
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
	defer host.CloseResult(&resultErr)
	ctx = host.Context
	business := runtime.Compose(host.Access, host.Monitoring).Configure(cfg)
	requests, stopRequests := replicabudget.Requests(ctx)
	defer stopRequests()
	diagnosticreport.SetMetric(reporter, "quota.pg_connections", int64(host.Pool.Config().MaxConns))
	if host.Monitoring != nil {
		diagnosticreport.SetMetric(reporter, "quota.ch_connections", int64(host.Monitoring.ConnectionLimit()))
	}
	if obj := cfg.Architecture.ObjectRequests(); obj.Attached() {
		diagnosticreport.SetMetric(reporter, "quota.s3_inflight", int64(obj.Capacity()))
	}
	servers := []*http.Server{}
	listeners := []net.Listener{}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	handlers := map[string]http.Handler{}
	split := cfg.Architecture != nil && cfg.Architecture.DisplaySplit
	if role == "display-assets" && !split {
		return errors.New("split display must be enabled")
	}
	if role == "display-runtime" {
		if split {
			network, e := privateapi.Open(cfg.Architecture)
			if e != nil {
				return e
			}
			defer network.Close()
			business.Displays.WithReadExecutor(network.ExecuteRead)
		}
		shell := displayhttp.RuntimeHandler(business.Displays)
		handlers[cfg.Display.RuntimeAddr] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			shell.ServeHTTP(w, r.WithContext(privateapi.WithTrace(r.Context(), rand.Text())))
		})
	}
	if role == "display-assets" || !split {
		handlers[cfg.Display.AssetAddr] = displayhttp.AssetHandler(business.Displays)
	}
	var limitHandlers func(http.Handler) http.Handler
	if cfg.Architecture != nil {
		limit := cfg.Architecture.Inflight(role)
		limitHandlers = privateapi.LimitHandlers(limit)
		diagnosticreport.SetMetric(reporter, "quota.http_inflight", int64(limit))
	}
	for addr, handler := range handlers {
		if limitHandlers != nil {
			handler = limitHandlers(handler)
		}
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
		servers = append(servers, &http.Server{Handler: diagnosticreport.Readiness(handler, reporter), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return requests }})
	}
	done := make(chan error, 2)
	for i, s := range servers {
		go func() { done <- s.Serve(listeners[i]) }()
	}
	reporter.Ready()
	logger.Info("display endpoints ready", "role", role, "listeners", len(listeners))
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
