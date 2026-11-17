package parsing_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type ingestionReport struct {
	mu      sync.Mutex
	failed  map[string]bool
	healthy map[string]bool
	lanes   map[string]bool
}

func (*ingestionReport) Starting()               {}
func (*ingestionReport) Ready()                  {}
func (*ingestionReport) Unconfigured()           {}
func (*ingestionReport) Failed(string)           {}
func (*ingestionReport) Begin(string, time.Time) {}
func (*ingestionReport) Idle(string, time.Time)  {}
func (*ingestionReport) Stopping()               {}
func (r *ingestionReport) End(lane, reason string, _ time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason != "" {
		r.lanes[lane] = true
	}
}
func (r *ingestionReport) Dependency(id string, ready bool, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ready {
		r.healthy[id] = true
	} else {
		r.failed[id] = true
	}
}
func (r *ingestionReport) seen(kind, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch kind {
	case "failed":
		return r.failed[id]
	case "healthy":
		return r.healthy[id]
	default:
		return r.lanes[id]
	}
}

func TestIngestionLaneDiagnosticsDoNotBlockTakeover(t *testing.T) {
	f := setupIngestion(t)
	var mode atomic.Int32
	mode.Store(1)
	upstream := os.Getenv("INFRAPRO_PARSING_URL")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode.Load() == 1 && r.URL.Path == "/v1/identify" || mode.Load() == 2 && r.URL.Path == "/v1/parse" {
			http.Error(w, "synthetic parser outage with private upstream detail", 503)
			return
		}
		request, e := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.Path, r.Body)
		if e != nil {
			http.Error(w, "request", 500)
			return
		}
		request.Header = r.Header.Clone()
		response, e := http.DefaultClient.Do(request)
		if e != nil {
			http.Error(w, "upstream", 503)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		io.Copy(w, response.Body)
	}))
	defer server.Close()
	f.ps.WithOptions(f.options(server.URL))
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	report := &ingestionReport{failed: map[string]bool{}, healthy: map[string]bool{}, lanes: map[string]bool{}}
	work, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- f.s.RunWorker(work, report) }()
	defer func() { cancel(); must(t, <-done) }()
	await := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		var state string
		_ = f.env.Pool.QueryRow(ctx, `SELECT jsonb_build_object('receipts',(SELECT jsonb_agg(jsonb_build_object('state',state,'error',error_code)) FROM ingestion_receipts),'decisions',(SELECT jsonb_agg(jsonb_build_object('state',state,'error',error_code,'attempts',attempts)) FROM ingestion_identity_decisions),'attempts',(SELECT jsonb_agg(jsonb_build_object('stage',stage,'start',started_at,'finish',finished_at)) FROM ingestion_attempts),'plans',(SELECT jsonb_agg(jsonb_build_object('state',state,'error',error_code,'attempts',attempts)) FROM ingestion_parse_plans))::text`).Scan(&state)
		t.Fatalf("diagnostic condition timed out: %s", state)
	}
	await(func() bool { return report.seen("failed", "parser") && report.seen("lane", "identify") })
	var receipts, ack int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_receipts`).Scan(&receipts))
	await(func() bool {
		_ = f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_ack_outbox WHERE state='acknowledged'`).Scan(&ack)
		return ack == 1
	})
	if receipts != 1 {
		t.Fatal("parser outage blocked durable takeover")
	}
	mode.Store(2)
	await(func() bool { return report.seen("lane", "parse") })
	mode.Store(0)
	var results int
	await(func() bool {
		_ = f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results)
		return results == 2
	})
	for _, dependency := range []string{"postgres", "s3", "parser"} {
		if !report.seen("healthy", dependency) {
			t.Fatal("real dependency was not reported", dependency)
		}
	}
	var attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT attempts FROM ingestion_identity_decisions`).Scan(&attempts))
	if attempts != 2 {
		t.Fatal("a single retry was claimed by multiple workers", attempts)
	}
	var delay float64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT extract(epoch FROM max(started_at)-min(started_at))::double precision FROM ingestion_attempts WHERE stage='identify'`).Scan(&delay))
	if delay < 1 {
		t.Fatal("identification backoff was bypassed", delay)
	}
	t.Log("real identify/parse outages report dependency and lane failure; receipt/ACK continue; one retry uses exactly two identity attempts and respects backoff before two successful results")
}
