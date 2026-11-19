package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger, reporters ...diagnosticreport.Sink) (resultErr error) {
	reporter := diagnosticreport.Optional(reporters)
	role := ""
	var network *privateapi.Network
	var err error
	if cfg.Architecture != nil {
		role = cfg.Architecture.Role
		network, err = privateapi.Open(cfg.Architecture)
		if err != nil {
			return err
		}
		defer network.Close()
	}
	addr := cfg.HTTPAddr
	var public http.Handler
	if role == config.Gateway {
		var capacity int
		for _, o := range config.Owners {
			capacity += cfg.Architecture.Inflight(o)
		}
		limit := min(capacity, cfg.Architecture.Inflight(role))
		diagnosticreport.SetMetric(reporter, "quota.http_inflight", int64(limit))
		public = privateapi.LimitHandlers(limit)(httptransport.Gateway(cfg, network, logger))
	} else {
		localIdentity := role == "" || role == config.Identity || role == "compat-api"
		var resources *runtime.Resources
		if localIdentity {
			resources, err = runtime.Open(ctx, cfg, logger, true)
		} else {
			resources, err = runtime.OpenWorker(ctx, cfg, logger)
		}
		reporter.Dependency("bootstrap", err == nil, diagnosticreport.Result(err))
		if err != nil {
			return err
		}
		defer resources.CloseResult(&resultErr)
		ctx = resources.Context
		diagnosticreport.SetMetric(reporter, "quota.pg_connections", int64(resources.Pool.Config().MaxConns))
		if resources.Monitoring != nil {
			diagnosticreport.SetMetric(reporter, "quota.ch_connections", int64(resources.Monitoring.ConnectionLimit()))
		}
		if localIdentity {
			jobs, cancelJobs := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); resources.Identity.RunJobs(jobs) }()
			defer func() {
				cancelJobs()
				<-done
				flush, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
				defer cancel()
				if resources.Identity.Flush(flush) != nil {
					logger.Warn("final activity flush incomplete")
				}
			}()
		}
		owner := role
		if owner == "compat-api" {
			owner = ""
		}
		verify := privateapi.LocalVerify(resources.Identity)
		if !localIdentity {
			verify = network.Verify
		}
		ready := resources.Pool.Ping
		if localIdentity {
			ready = resources.Identity.Ready
		}
		private := http.NewServeMux()
		public = httptransport.OwnerRouter(logger, resources.Identity, cfg, owner, verify, func(b runtime.Business) {
			if owner == "" {
				privateapi.CompatibilityTrials(resources.Pool, b.DerivedMonitoring)
			}
			if resources.Monitoring != nil && (role == config.Query || owner == "") {
				base := ready
				ready = func(ctx context.Context) error {
					if err := base(ctx); err != nil {
						return err
					}
					return b.DerivedMonitoring.Ready(ctx)
				}
			}
			if network != nil {
				configurePrivateBusiness(private, b, network, cfg)
			}
		}, resources.Monitoring)
		if limit := cfg.Architecture.ObjectRequests(); limit.Attached() {
			diagnosticreport.SetMetric(reporter, "quota.s3_inflight", int64(limit.Capacity()))
		}
		if network != nil {
			addr = cfg.Architecture.Listener(role)
			if localIdentity {
				private.Handle(privateapi.SessionPath, privateapi.VerifyHandler(resources.Identity))
			}
			private.Handle(httptransport.ReadyPath, privateapi.Allow([]string{config.Gateway}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var empty struct{}
				if !privateapi.Read(w, r, 64, &empty) {
					return
				}
				probe, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				privateapi.Write(w, ready(probe) == nil)
			})))
			ownerHandler := httptransport.FromGateway(public)
			public = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/internal/") {
					private.ServeHTTP(w, r)
					return
				}
				ownerHandler.ServeHTTP(w, r)
			})
			limit := cfg.Architecture.Inflight(role)
			public = network.RegisteredPeer(privateapi.LimitHandlers(limit)(public))
			diagnosticreport.SetMetric(reporter, "quota.http_inflight", int64(limit))
		}
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	if role != "" && role != config.Gateway {
		listener = tls.NewListener(listener, network.TLS)
	}
	requests, stopRequests := replicabudget.Requests(ctx)
	defer stopRequests()
	server := &http.Server{
		Handler:           diagnosticreport.Readiness(public, reporter),
		BaseContext:       func(net.Listener) context.Context { return requests },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if role == config.Gateway {
		server.ReadTimeout = 0
		server.WriteTimeout = 0
	} // domain/proxy deadlines include large streamed transfers.
	servers := []*http.Server{server}
	listeners := []net.Listener{listener}
	if role == config.Gateway && cfg.Architecture.Endpoints[config.Gateway] != "" {
		probeListener, e := net.Listen("tcp", cfg.Architecture.Listener(config.Gateway))
		if e != nil {
			_ = listener.Close()
			return fmt.Errorf("listen on private gateway probe: %w", e)
		}
		listeners = append(listeners, tls.NewListener(probeListener, network.TLS))
		servers = append(servers, &http.Server{Handler: network.RegisteredPeer(httptransport.GatewayProbes(network)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 6 * time.Second, IdleTimeout: 60 * time.Second})
	}
	done := make(chan error, len(servers))
	for i, srv := range servers {
		go func() { done <- srv.Serve(listeners[i]) }()
	}
	reporter.Ready()
	reporter.Idle("http", time.Time{})
	logger.Info("api listening", "address", listener.Addr().String())
	select {
	case err = <-done:
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		if e := srv.Shutdown(shutdown); e != nil {
			_ = srv.Close()
			if err == nil {
				err = fmt.Errorf("shutdown api: %w", e)
			}
		}
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
