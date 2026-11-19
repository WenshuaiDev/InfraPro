package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if e := run(); e != nil {
		slog.Error("event relay stopped", "error", e.Error())
		os.Exit(1)
	}
}
func run() (result error) {
	action := "run"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}
	if action == "broker-health" {
		client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, e := client.Get("http://127.0.0.1:8222/healthz?js-enabled-only=true")
		if e != nil {
			return errors.New("broker health listener unavailable")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errors.New("JetStream not ready")
		}
		return nil
	}
	var r *diagnosticreport.Reporter
	if action == "run" {
		r = diagnosticreport.FromEnv()
		defer func() {
			if result != nil {
				r.Failed("dependency_unavailable")
			}
			r.Close()
		}()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	c, e := eventbus.Load(os.Getenv("INFRAPRO_EVENTS_FILE"))
	if e != nil {
		return e
	}
	db, e := config.LoadDatabase(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pc, e := pgxpool.ParseConfig(db.URL)
	if e != nil {
		return errors.New("invalid event database configuration")
	}
	pc.MaxConns, e = config.WorkerPoolLimit("event-relay", 4)
	if e != nil {
		return e
	}
	if action == "run" {
		diagnosticreport.SetMetric(r, "quota.pg_connections", int64(pc.MaxConns))
	}
	pc.ConnConfig.ConnectTimeout = 3 * time.Second
	var arch *config.Architecture
	if action == "run" {
		arch, e = config.WorkerArchitecture("event-relay")
		if e != nil {
			return e
		}
	}
	slot, e := replicabudget.Open(ctx, pc, arch.SlotKey())
	if e != nil {
		return errors.New("event database unavailable")
	}
	defer slot.CloseResult(&result)
	p, ctx := slot.Pool, slot.Context
	if e = migrations.Check(ctx, p); e != nil {
		return e
	}
	if action == "run" {
		r.Ready()
		return eventbus.RunRelay(ctx, p, c, r)
	}
	if action == "status" {
		rows, e := p.Query(ctx, `SELECT jsonb_build_object('protocol',(SELECT to_jsonb(p) FROM event_protocol p),'consumers',(SELECT jsonb_agg(to_jsonb(s) ORDER BY consumer) FROM event_status s),'ownership',(SELECT jsonb_agg(to_jsonb(o) ORDER BY role) FROM dispatch_ownership o),'pressure',(SELECT jsonb_agg(to_jsonb(m) ORDER BY family) FROM event_pressure m))`)
		if e != nil {
			return e
		}
		defer rows.Close()
		if rows.Next() {
			var v json.RawMessage
			if e = rows.Scan(&v); e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(v)
		}
		return rows.Err()
	}
	if action == "provision" {
		st, e := eventbus.ProtocolState(ctx, p)
		if e != nil {
			return e
		}
		ctx, finish := context.WithTimeout(ctx, 30*time.Second)
		defer finish()
		if c.Mode != "jetstream" {
			return errors.New("provision requires JetStream mode")
		}
		return eventbus.Provision(ctx, c, st.Epoch)
	}
	op := eventbus.Operation{Action: action}
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	flags.StringVar(&op.Reason, "reason", "", "audited operational reason")
	flags.StringVar(&op.Epoch, "epoch", "", "expected current restore epoch")
	flags.StringVar(&op.Role, "role", "", "dispatch role")
	flags.StringVar(&op.Mode, "mode", "", "split, compat, draining or frozen")
	flags.Int64Var(&op.Expected, "expected", 0, "expected dispatch generation")
	flags.StringVar(&op.Consumer, "consumer", "", "durable consumer name")
	flags.StringVar(&op.ID, "event", "", "quarantined event ID")
	flags.Int64Var(&op.SoftLimit, "soft", 0, "pending row soft threshold per stream")
	flags.Int64Var(&op.HardLimit, "hard", 0, "pending row hard threshold per stream")
	if e = flags.Parse(os.Args[2:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	st, e := eventbus.Operate(ctx, p, op)
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(st)
}
