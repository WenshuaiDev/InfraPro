package parsing_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/google/uuid"
)

func TestArchitectureRecordedDispatchRace(t *testing.T) {
	f := setupIngestion(t)
	_, err := eventbus.Operate(ctx, f.env.Pool, eventbus.Operation{Action: "enable", Reason: "recorded dispatch evidence"})
	must(t, err)
	batch, err := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, err)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("fixture reporting failed")
	}
	_, err = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, err)
	var receipt string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_receipts`).Scan(&receipt))
	f.s.ConfigureDispatch(time.Second, nil)
	wrong, err := f.s.AttemptDispatch(ctx, "identify", uuid.NewString(), receipt, 1)
	must(t, err)
	if !wrong.Exists || wrong.Scope != f.tenant || !wrong.Pending {
		t.Fatal("corrupt scope did not resolve the actual pending fact", wrong)
	}

	claimed, err := f.s.IdentifyOne(ctx)
	must(t, err)
	if claimed {
		t.Fatal("legacy lane executed an event-owned receipt")
	}
	entered, switched := make(chan struct{}), make(chan struct{})
	stale := make(chan error, 1)
	go func() {
		_, e := dispatch.Try(ctx, f.env.Pool, "parsing-dispatch-worker", "compat", func(old context.Context) error {
			close(entered)
			<-switched
			_, failure := f.s.IdentifyReceipt(old, f.tenant, receipt, 1)
			return failure
		})
		stale <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old mode did not acquire")
	}
	_, err = eventbus.Operate(ctx, f.env.Pool, eventbus.Operation{Action: "switch", Mode: "split", Role: "parsing-dispatch-worker", Expected: 1, Reason: "simulate topology transfer"})
	must(t, err)
	close(switched)
	if err = <-stale; !errors.Is(err, dispatch.ErrRetired) {
		t.Fatalf("stale generation reached domain: %v", err)
	}
	run := func(work func(context.Context) (bool, error)) {
		t.Helper()
		var g sync.WaitGroup
		errs := make(chan error, 8)
		for range 8 {
			g.Go(func() {
				_, e := dispatch.Try(ctx, f.env.Pool, "parsing-dispatch-worker", "split", func(c context.Context) error { _, e := work(c); return e })
				errs <- e
			})
		}
		g.Wait()
		close(errs)
		for e := range errs {
			must(t, e)
		}
	}
	run(func(c context.Context) (bool, error) { return f.s.IdentifyReceipt(c, f.tenant, receipt, 1) })
	var decisions int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_identity_decisions WHERE state='identified' AND generation=1`).Scan(&decisions))
	if decisions != 1 {
		t.Fatal("identity selection", decisions)
	}
	units, err := f.env.Pool.Query(ctx, `SELECT id::text FROM ingestion_units ORDER BY unit_key`)
	must(t, err)
	var ids []string
	for units.Next() {
		var id string
		must(t, units.Scan(&id))
		ids = append(ids, id)
	}
	must(t, units.Err())
	units.Close()
	claimed, err = f.s.PlanOne(ctx)
	must(t, err)
	if claimed {
		t.Fatal("legacy lane executed a recorded unit")
	}
	for _, id := range ids {
		run(func(c context.Context) (bool, error) { return f.s.PlanUnit(c, f.tenant, id, 1) })
	}
	plans, err := f.env.Pool.Query(ctx, `SELECT id::text FROM ingestion_parse_plans ORDER BY id`)
	must(t, err)
	ids = nil
	for plans.Next() {
		var id string
		must(t, plans.Scan(&id))
		ids = append(ids, id)
	}
	must(t, plans.Err())
	plans.Close()
	claimed, err = f.s.ParseOne(ctx)
	must(t, err)
	if claimed {
		t.Fatal("legacy lane executed a recorded plan")
	}
	for _, id := range ids {
		run(func(c context.Context) (bool, error) { return f.s.ParsePlan(c, f.tenant, id, 1) })
	}
	var succeeded, publication int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ingestion_results),(SELECT count(*) FROM monitoring_publications)`).Scan(&succeeded, &publication))
	if len(ids) != 2 || succeeded != 2 || publication != 0 {
		t.Fatal("multi-frame race changed effects", len(ids), succeeded, publication)
	}
}

func TestArchitectureAmbiguousParserReplyKeepsIdentity(t *testing.T) {
	f := setupIngestion(t)
	batch, err := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, err)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("fixture reporting failed")
	}
	_, err = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, err)
	_, err = f.s.IdentifyOne(ctx)
	must(t, err)
	_, err = f.s.PlanOne(ctx)
	must(t, err)
	var plan string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_parse_plans`).Scan(&plan))
	var received []string
	var lock sync.Mutex
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, e := http.NewRequest(r.Method, os.Getenv("INFRAPRO_PARSING_URL")+r.URL.Path, r.Body)
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		request.Header = r.Header.Clone()
		response, e := http.DefaultClient.Do(request)
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		defer response.Body.Close()
		body, e := io.ReadAll(response.Body)
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		if response.StatusCode == 200 && r.URL.Path == "/v1/parse" {
			var v wire.Response
			if e = json.Unmarshal(body, &v); e != nil {
				t.Error(e)
				w.WriteHeader(500)
				return
			}
			lock.Lock()
			received = append(received, v.ExecutionID)
			first := len(received) == 1
			lock.Unlock()
			if first {
				// The remote result exists. This generation loses its lease before
				// it can select the reply, as it would after a dispatcher crash.
				_, e = f.env.Pool.Exec(ctx, `UPDATE ingestion_parse_plans SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, plan)
				if e != nil {
					t.Error(e)
					w.WriteHeader(500)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(body)
	}))
	defer proxy.Close()
	f.ps.WithOptions(f.options(proxy.URL))
	for expected := 0; expected <= 1; expected++ {
		claimed, e := f.s.ParsePlan(ctx, f.tenant, plan, 1)
		must(t, e)
		if !claimed {
			t.Fatal("plan was not claimed", expected)
		}
		var selected int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results WHERE plan_id=$1`, plan).Scan(&selected))
		if selected != expected {
			t.Fatal("expired response changed selected result", selected, expected)
		}
	}
	var attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(DISTINCT execution_id) FROM ingestion_attempts WHERE stage='parse' AND target_id=$1`, plan).Scan(&attempts))
	lock.Lock()
	defer lock.Unlock()
	if attempts != 2 || len(received) != 2 || received[0] == "" || received[0] != received[1] {
		t.Fatal("machine identity changed or attempt audit lost", attempts, received)
	}
}

// A PG repair claimant is paused after taking its lease. A real JetStream
// delivery of the same event and a retired compatibility owner then arrive.
func TestArchitectureBrokerAndPGRace(t *testing.T) {
	if os.Getenv("INFRAPRO_ARCH_WORKER_CHECKS") != "1" {
		t.Skip("private broker and relay fixtures required")
	}
	f := setupIngestion(t)
	state, e := eventbus.Operate(ctx, f.env.Pool, eventbus.Operation{Action: "enable", Reason: "race one retained receipt"})
	must(t, e)
	root := os.Getenv("INFRAPRO_TEST_REPO_ROOT")
	dir := t.TempDir()
	makeConfig := exec.Command("node", filepath.Join(root, "scripts", "architecture-event-lab.mjs"), dir, "arcdispatch", state.Epoch)
	makeConfig.Dir = root
	if output, e := makeConfig.CombinedOutput(); e != nil {
		t.Fatalf("create isolated broker: %s %v", output, e)
	}
	var lab struct{ Config string }
	raw, e := os.ReadFile(filepath.Join(dir, "lab.json"))
	must(t, e)
	must(t, json.Unmarshal(raw, &lab))
	broker := exec.Command(filepath.Join(root, "build", "events", runtime.GOOS+"-"+runtime.GOARCH, "nats-server"), "-c", lab.Config)
	must(t, broker.Start())
	defer func() { _ = broker.Process.Kill(); _ = broker.Wait() }()
	admin, e := eventbus.Load(filepath.Join(dir, "event-admin", "events.json"))
	must(t, e)
	archWait(t, 10*time.Second, func() bool { return eventbus.Provision(ctx, admin, state.Epoch) == nil }, "fresh broker streams")
	relay, e := eventbus.Load(filepath.Join(dir, "event-relay", "events.json"))
	must(t, e)
	workerPath := filepath.Join(dir, "parsing-dispatch-worker", "events.json")
	worker, e := eventbus.Load(workerPath)
	must(t, e)
	worker.FallbackScanMS = 250
	worker.BackoffMS = 50
	archJSON(t, workerPath, worker)
	t.Setenv("INFRAPRO_EVENTS_FILE", workerPath)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("fixture reporting failed")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	var receipt string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_receipts`).Scan(&receipt))
	f.s.ConfigureDispatch(time.Second, nil)
	run, cancel := context.WithTimeout(ctx, 60*time.Second)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	oldReady := make(chan struct{})
	letOldFinish := make(chan struct{})
	older := make(chan error, 1)
	wg.Go(func() {
		_, err := dispatch.Try(run, f.env.Pool, "parsing-dispatch-worker", "compat", func(stale context.Context) error {
			close(oldReady)
			select {
			case <-letOldFinish:
			case <-run.Done():
				return run.Err()
			}
			_, err := f.s.IdentifyReceipt(stale, f.tenant, receipt, 1)
			return err
		})
		older <- err
	})
	select {
	case <-oldReady:
	case <-run.Done():
		t.Fatal(run.Err())
	}
	_, e = eventbus.Operate(ctx, f.env.Pool, eventbus.Operation{Action: "switch", Role: "parsing-dispatch-worker", Mode: "split", Expected: 1, Reason: "retire pending compatibility reply"})
	must(t, e)
	pgClaimed := make(chan struct{})
	letNewFinish := make(chan struct{})
	var calls atomic.Int32
	wg.Go(func() {
		_, _ = dispatch.Try(run, f.env.Pool, "parsing-dispatch-worker", "split", func(active context.Context) error {
			return eventbus.RunConsumer(active, f.env.Pool, "identify", func(c context.Context, v eventbus.Event) error {
				if v.AggregateID != receipt {
					return eventbus.ErrBadReference
				}
				if calls.Add(1) == 1 {
					close(pgClaimed)
				}
				select {
				case <-letNewFinish:
				case <-c.Done():
					return c.Err()
				}
				status, err := f.s.AttemptDispatch(c, "identify", v.Scope, v.AggregateID, v.Generation)
				if err == nil && (!status.Exists || status.Pending || status.Scope != v.Scope || status.Generation != v.Generation) {
					return eventbus.ErrDeferred
				}
				return err
			})
		})
	})
	select {
	case <-pgClaimed:
	case <-run.Done():
		t.Fatal("PG repair failed to claim queued receipt")
	}
	wg.Go(func() { _ = eventbus.RunRelay(run, f.env.Pool, relay, diagnosticreport.Optional(nil)) })
	archWait(t, 15*time.Second, func() bool {
		var shared bool
		must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_consumptions c JOIN event_outbox o USING(restore_epoch,event_id) WHERE c.consumer='identify' AND o.aggregate_id=$1 AND c.via='pg' AND c.duplicates>=1 AND o.state='sent')`, receipt).Scan(&shared))
		return shared
	}, "broker and PG have accepted the same work")
	close(letOldFinish)
	select {
	case e = <-older:
		if !errors.Is(e, dispatch.ErrRetired) {
			t.Fatal("old owner was able to commit", e)
		}
	case <-run.Done():
		t.Fatal(run.Err())
	}
	claimed, e := f.s.IdentifyOne(ctx)
	must(t, e)
	if claimed {
		t.Fatal("old native scanner stole recorded receipt")
	}
	close(letNewFinish)
	archWait(t, 10*time.Second, func() bool {
		var complete int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM event_work WHERE consumer='identify' AND state='done'`).Scan(&complete))
		return complete == 1
	}, "selected current generation")
	var decisions, units, formal int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ingestion_identity_decisions WHERE state='identified'),(SELECT count(*) FROM ingestion_units),(SELECT count(*) FROM monitoring_publications)`).Scan(&decisions, &units, &formal))
	if calls.Load() != 1 || decisions != 1 || units != 2 || formal != 0 {
		t.Fatal("duplicate business effect", calls.Load(), decisions, units, formal)
	}
}
