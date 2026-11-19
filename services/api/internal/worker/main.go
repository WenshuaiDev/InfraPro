// Package worker assembles the old and split topologies from the same durable
// claim functions. Each logical worker obtains its own revocable PG grant.
package worker

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/logging"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const custody = "ingest-custody-worker"
const parsingDispatch = "parsing-dispatch-worker"

type host struct {
	role, mode string
	cfg        config.Config
	resources  *runtime.Resources
	b          runtime.Business
	log        *slog.Logger
	report     diagnosticreport.Sink
}

func Main(role string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, role); err != nil {
		slog.Error("worker stopped", "role", role, "error", err)
		os.Exit(1)
	}
}
func Run(ctx context.Context, role string) (result error) {
	report := diagnosticreport.FromEnv()
	defer func() {
		if result != nil {
			report.Failed("dependency_unavailable")
		}
		report.Close()
	}()
	cfg, e := config.LoadRole(os.Getenv("INFRAPRO_CONFIG"), role)
	if e != nil {
		return e
	}
	mode := "split"
	if role == "monitoring-worker" || role == "ingest-worker" {
		mode = "compat"
	}
	if mode == "split" && cfg.Architecture == nil {
		return errors.New("split worker needs architecture configuration")
	}
	logger := logging.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	res, e := runtime.OpenWorker(ctx, cfg, logger)
	if e != nil {
		return e
	}
	defer res.CloseResult(&result)
	ctx = res.Context
	h := host{role: role, mode: mode, cfg: cfg, resources: res, b: runtime.Compose(res.Access, res.Monitoring).Configure(cfg), log: logger, report: report}
	var fallback time.Duration
	if mode == "split" {
		fallback = eventbus.FallbackInterval()
	}
	h.b.Ingestion.ConfigureDispatch(fallback, report)
	if role == parsingDispatch || role == "ingest-worker" || role == "monitoring-worker" {
		diagnosticreport.SetMetric(report, "quota.parser_slots", parsing.ExecutorSlots)
	}
	diagnosticreport.SetMetric(report, "quota.pg_connections", int64(res.Pool.Config().MaxConns))
	if res.Monitoring != nil {
		diagnosticreport.SetMetric(report, "quota.ch_connections", int64(res.Monitoring.ConnectionLimit()))
	}
	if limit := cfg.Architecture.ObjectRequests(); limit.Attached() {
		diagnosticreport.SetMetric(report, "quota.s3_inflight", int64(limit.Capacity()))
	}
	if res.Monitoring != nil {
		if e = h.b.DerivedMonitoring.Ready(ctx); e != nil {
			return e
		}
	}
	h.b.DisplayReads.WithWorkerLimit(max(1, cfg.Derived.HistoricalWorkers))
	return h.run(ctx)
}
func (h host) run(ctx context.Context) error {
	group, work := errgroup.WithContext(ctx)
	add := func(role string, fn func(context.Context) error) {
		group.Go(func() error {
			return dispatch.Run(work, h.resources.Pool, role, h.mode, func(leased context.Context) error {
				h.report.Ready()
				return fn(leased)
			})
		})
	}
	ingestCompat := h.role == "ingest-worker"
	monitorCompat := h.role == "monitoring-worker"
	if h.role == custody || ingestCompat {
		if e := h.b.Ingestion.Ready(ctx); e != nil {
			return e
		}
		add(custody, func(c context.Context) error { return h.b.Ingestion.RunCustody(c, h.report) })
	}
	if h.role == parsingDispatch || ingestCompat || (monitorCompat && h.cfg.Parsing.Enabled) {
		add(parsingDispatch, func(c context.Context) error {
			g, c := errgroup.WithContext(c)
			if !monitorCompat {
				g.Go(func() error { return h.b.Ingestion.RunDispatch(c, h.report) })
				if !ingestCompat {
					h.ingestionConsumers(c, g)
				}
			}
			if !ingestCompat {
				g.Go(func() error { return h.b.Parsing.RunWorker(c, h.report) })
			}
			return g.Wait()
		})
	}
	for _, r := range []monitoring.WorkerRole{monitoring.PublishWorker, monitoring.IncrementalWorker, monitoring.HistoryWorker, monitoring.ReadWorker} {
		if h.role != string(r) && !monitorCompat {
			continue
		}
		if h.resources.Monitoring == nil && !monitorCompat {
			return errors.New("monitoring storage is not configured")
		}
		add(string(r), func(c context.Context) error {
			g, c := errgroup.WithContext(monitoring.WorkerContext(c, r))
			if h.resources.Monitoring != nil {
				g.Go(func() error { return h.b.Monitoring.RunWorker(c, h.log, h.report) })
			}
			if r == monitoring.ReadWorker {
				g.Go(func() error { return h.b.DisplayReads.RunWorker(c, "display-reads-"+uuid.NewString(), h.report) })
				if !monitorCompat {
					h.changeConsumers(c, g, "display-cache", "query-cache")
				}
			}
			if r == monitoring.IncrementalWorker && !monitorCompat {
				h.changeConsumers(c, g, "derived", "summary")
				g.Go(func() error { return h.serveTrials(c) })
			}
			if h.resources.Monitoring == nil && r != monitoring.ReadWorker {
				<-c.Done()
			}
			return g.Wait()
		})
	}
	return group.Wait()
}

func (h host) serveTrials(ctx context.Context) error {
	a := h.cfg.Architecture
	addr := a.Listener(string(monitoring.IncrementalWorker))
	if addr == "" {
		return fmt.Errorf("missing private endpoint for %s", monitoring.IncrementalWorker)
	}
	network, e := privateapi.Open(a)
	if e != nil {
		return e
	}
	defer network.Close()
	listener, e := tls.Listen("tcp", addr, network.TLS)
	if e != nil {
		return e
	}
	mux := http.NewServeMux()
	mux.Handle(privateapi.TrialPath, privateapi.TrialHandler(h.b.DerivedMonitoring, h.cfg.Derived.TrialDeadline))
	timeout := h.cfg.Derived.TrialDeadline + 5*time.Second
	limit := a.Inflight(h.role)
	diagnosticreport.SetMetric(h.report, "quota.http_inflight", int64(limit))
	server := &http.Server{Handler: network.RegisteredPeer(privateapi.LimitHandlers(limit)(mux)), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: timeout, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	h.report.Idle("trial-http", time.Time{})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), h.cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
		<-done
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
