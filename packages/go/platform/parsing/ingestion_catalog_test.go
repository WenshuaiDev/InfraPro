package parsing_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
)

func TestIngestionCatalogAvailabilityAndLifecycle(t *testing.T) {
	f := setupIngestion(t)
	platform := c.PlatformScope()
	caps, e := f.ps.Capabilities(ctx, f.actor(), platform, "ingress-protocols", c.Filter{Limit: 100})
	must(t, e)
	chosen := caps.Items[0]
	var protocol i.Protocol
	must(t, json.Unmarshal(chosen.Entry, &protocol))
	checkProtocol := func(newUse bool, want string) {
		t.Helper()
		tx, e := f.env.Pool.Begin(ctx)
		must(t, e)
		_, e = f.ps.IngressProtocolTx(ctx, tx, protocol.ID, protocol.Version, newUse)
		tx.Rollback(ctx)
		if want == "" {
			must(t, e)
		} else {
			ingestionError(t, e, want)
		}
	}
	for _, state := range []string{"deprecated", "blocked", "enabled"} {
		result, e := f.ps.SetCapabilityState(ctx, f.actor(), "ingress-protocols", chosen.ID, parse.StateInput{Meta: f.meta(t), Revision: chosen.Revision, State: state})
		must(t, e)
		chosen.Revision = result.Revision
		if state == "deprecated" {
			checkProtocol(false, "")
			checkProtocol(true, "method_not_adoptable")
		}
		if state == "blocked" {
			checkProtocol(false, "method_blocked")
			checkProtocol(true, "method_blocked")
		}
	}
	var mode atomic.Int32
	upstream := os.Getenv("INFRAPRO_PARSING_URL")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ingestion-capabilities" && mode.Load() == 1 {
			http.Error(w, "synthetic unavailable", 503)
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
		body, e := io.ReadAll(response.Body)
		if e != nil {
			http.Error(w, "read", 500)
			return
		}
		if r.URL.Path == "/v1/ingestion-capabilities" && mode.Load() == 2 {
			var catalog i.Catalog
			if json.Unmarshal(body, &catalog) != nil {
				http.Error(w, "catalog", 500)
				return
			}
			catalog.Compatibilities = catalog.Compatibilities[:len(catalog.Compatibilities)-1]
			body, _ = json.Marshal(catalog)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		w.Write(body)
	}))
	defer server.Close()
	f.ps.WithOptions(f.options(server.URL))
	var before string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(string_agg(entry::text,'' ORDER BY id)) FROM parsing_method_versions`).Scan(&before))
	for _, fault := range []int32{1, 2} {
		mode.Store(fault)
		result, e := f.ps.SyncIngestion(ctx, f.actor(), f.meta(t))
		must(t, e)
		if result.State != "failed" {
			t.Fatal("incomplete/unavailable ingress catalog accepted")
		}
		retained, e := f.ps.Capabilities(ctx, f.actor(), platform, "ingress-protocols", c.Filter{Limit: 100})
		must(t, e)
		if len(retained.Items) != len(caps.Items) {
			t.Fatal("historical catalog removed")
		}
		for _, v := range retained.Items {
			if v.Available {
				t.Fatal("unavailable catalog still advertised as usable")
			}
		}
		checkProtocol(true, "artifact_unavailable")
		_, e = f.ps.SetCapabilityState(ctx, f.actor(), "ingress-protocols", chosen.ID, parse.StateInput{Meta: f.meta(t), Revision: chosen.Revision, State: "enabled"})
		ingestionError(t, e, "artifact_unavailable")
	}
	// The old method-test workflow still calls the real contract-1 parser.
	sample := f.sample(t, platform, f.actor(), "measurement.bin", "binary")
	run := f.methodRun(t, platform, f.actor(), sample, "1.0.0")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	f.checkRun(t, platform, f.actor(), run.ID, "succeeded", "")
	result, _, e := f.ps.Result(ctx, f.actor(), platform, run.ID, "1", false, 0, 100)
	must(t, e)
	resultJSON, e := json.Marshal(result)
	must(t, e)
	if !bytes.Contains(resultJSON, []byte("12.34")) {
		t.Fatal("legacy numeric result changed")
	}
	var after string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(string_agg(entry::text,'' ORDER BY id)) FROM parsing_method_versions`).Scan(&after))
	if before != after {
		t.Fatal("ingress failure modified measurement identities")
	}
	mode.Store(0)
	result2, e := f.ps.SyncIngestion(ctx, f.actor(), f.meta(t))
	must(t, e)
	if result2.State != "succeeded" {
		t.Fatal(result2.ErrorCode)
	}
	checkProtocol(true, "")
	t.Log("deprecated/blocked/new-use gates, unavailable and partial catalog retention, old contract-1 manual replay, and recovery verified")
}
