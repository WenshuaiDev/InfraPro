package bootstrap_test

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
)

func TestArchitectureColdStartOutages(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL and Redis")
	}
	env, err := testsupport.New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	if _, err = env.Service.Bootstrap(t.Context(), "outageadmin", "故障隔离验收", "ArchitectureVerification492", identity.Meta{}); err != nil {
		t.Fatal(err)
	}
	// Both clients are configured; neither server is listening. Identity must
	// be able to start, sign in, and authorize ordinary PG-only business reads.
	env.Config.Monitoring = config.Monitoring{Replicas: []string{address(t)}, Database: "architecture_outage", Policy: "monitoring_tiered", User: "default", ObjectEndpoint: "http://" + address(t), ObjectAccessKey: "fixture", ObjectSecretKey: "fixture", Bucket: "architecture-outage", TempDirectory: t.TempDir()}
	lab := startLab(t, env, true)
	person := newBrowser(lab.gateway, env.Config.TrustedOrigins[0])
	person.prepare(t)
	if r := person.do(t, "POST", "/api/v1/auth/login", `{"username":"outageadmin","password":"ArchitectureVerification492"}`); r.Code != 200 {
		t.Fatalf("identity cold start during CH outage: %d", r.Code)
	}
	if r := person.do(t, "GET", "/api/v1/platform/tenants", ""); r.Code != 200 {
		t.Fatalf("PG directory during CH outage: %d", r.Code)
	}
	if r := person.do(t, "GET", "/api/v1/me/notifications/summary", ""); r.Code != 200 {
		t.Fatalf("control during CH outage: %d", r.Code)
	}
	if r := person.do(t, "GET", "/readyz", ""); r.Code != 503 {
		t.Fatalf("aggregate lost a required dependency: %d", r.Code)
	}
	for _, role := range []string{"diagnostics-observer", config.Control} {
		local := lab.arch
		local.Role = role
		n, err := privateapi.Open(&local)
		if err != nil {
			t.Fatal(err)
		}
		cfg := n.TLS.Clone()
		cfg.ServerName = "api.infrapro.internal"
		cfg.ClientAuth = tls.NoClientCert
		client := http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 6 * time.Second}
		res, err := client.Get(lab.arch.Endpoints[config.Gateway] + "/internal/v1/upstreams")
		if err != nil {
			t.Fatal(err)
		}
		if role == "diagnostics-observer" {
			var state map[string]bool
			if err = json.NewDecoder(res.Body).Decode(&state); err != nil || res.StatusCode != 200 || !state[config.Identity] || !state[config.Control] || state[config.Query] {
				t.Fatalf("per-upstream readiness: %d %v", res.StatusCode, state)
			}
		} else if res.StatusCode != 404 {
			t.Fatal("unassigned certificate read deployment status")
		}
		res.Body.Close()
		client.CloseIdleConnections()
		n.Close()
	}
	lab.stop[config.Identity]()
	limit := time.Now().Add(5 * time.Second)
	for {
		r := person.do(t, "GET", "/api/v1/me/notifications", "")
		if r.Code == 503 {
			var out struct{ Code string }
			if json.Unmarshal(r.Body, &out) != nil || out.Code != "unavailable" {
				t.Fatalf("identity outage response: %s", r.Body)
			}
			break
		}
		if time.Now().After(limit) {
			t.Fatal("protected request accepted after identity shutdown")
		}
		time.Sleep(30 * time.Millisecond)
	}
	if r := person.do(t, "GET", "/healthz", ""); r.Code != 200 {
		t.Fatalf("gateway liveness after identity outage: %d", r.Code)
	}
}
