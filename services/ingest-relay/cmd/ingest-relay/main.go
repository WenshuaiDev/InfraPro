package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/ingest-relay/internal/server"
	"github.com/WenshuaiDev/InfraPro/services/ingest-relay/internal/spool"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "ingest-relay: startup or operation failed; inspect protected configuration and storage")
		os.Exit(1)
	}
}
func run() (result error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if result != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: ingest-relay migrate|serve|new-epoch|manifest CONFIG_FILE")
	}
	cfg, e := server.ReadConfig(os.Args[2])
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, e := pgxpool.New(ctx, cfg.DatabaseURL)
	if e != nil {
		return e
	}
	defer pool.Close()
	store := &spool.Store{Pool: pool, RelayID: cfg.RelayID, Budget: cfg.Budget}
	if os.Args[1] == "manifest" {
		var raw json.RawMessage
		e = pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg((r.envelope-'payload_base64')||jsonb_build_object('payload_present',r.payload IS NOT NULL,'acknowledged',a.receipt_id IS NOT NULL) ORDER BY r.receipt_id),'[]') FROM relay_receipts r LEFT JOIN relay_ack_receipts a USING(receipt_id)`).Scan(&raw)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(raw)
	}
	// Serving and epoch rotation are mutually exclusive; ordinary restarts keep epoch.
	lock, e := pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer lock.Release()
	var own bool
	if e = lock.QueryRow(ctx, `SELECT pg_try_advisory_lock(754040,hashtext(current_schema()))`).Scan(&own); e != nil || !own {
		return fmt.Errorf("relay process already active")
	}
	defer lock.Exec(context.Background(), `SELECT pg_advisory_unlock(754040,hashtext(current_schema()))`)
	switch os.Args[1] {
	case "migrate":
		return store.Migrate(ctx)
	case "new-epoch":
		// Explicit restore operation only; normal restarts never change epochs.
		_, e = pool.Exec(ctx, `UPDATE relay_state SET spool_epoch=$1 WHERE id=1`, uuid.NewString())
		return e
	case "serve":
	default:
		return fmt.Errorf("unknown command")
	}
	if e = store.Ready(ctx); e != nil {
		return e
	}
	s := server.New(cfg, store)
	defer s.Close()
	if e = s.Restore(ctx); e != nil {
		return e
	}
	t, e := cfg.TLS()
	if e != nil {
		return e
	}
	ln, e := tls.Listen("tcp", cfg.Address, t)
	if e != nil {
		return e
	}
	h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 35 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- h.Serve(ln) }()
	reporter.Ready()
	reporter.Idle("control", time.Now().Add(10*time.Second))
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stop, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()
			_ = h.Shutdown(stop)
			return nil
		case e := <-done:
			return e
		case <-tick.C:
			reporter.Begin("receivers", time.Now().Add(5*time.Second))
			e = store.Ready(ctx)
			reporter.Dependency("ingest-relay-postgres", e == nil, "dependency_unavailable")
			reason := ""
			if e != nil {
				reason = "dependency_unavailable"
			}
			reporter.End("receivers", reason, time.Now().Add(10*time.Second))
			reporter.Idle("control", time.Now().Add(10*time.Second))
			reporter.Begin("spool-maintenance", time.Now().Add(10*time.Second))
			if e = store.Cleanup(ctx); e != nil {
				fmt.Fprintln(os.Stderr, "ingest-relay: maintenance unavailable")
			}
			reason = ""
			if e != nil {
				reason = "dependency_unavailable"
			}
			reporter.End("spool-maintenance", reason, time.Now().Add(10*time.Second))
		}
	}
}
