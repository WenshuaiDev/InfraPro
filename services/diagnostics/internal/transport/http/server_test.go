package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/access"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type httpLab struct {
	t        *testing.T
	app      *Server
	server   *httptest.Server
	cookies  []*http.Cookie
	csrf     string
	auth     settings.Auth
	manifest settings.Manifest
	keys     settings.Keyring
	secret   string
	db       *store.Store
}

func newHTTPLab(t *testing.T) *httpLab {
	t.Helper()
	db, e := store.Open(testfixture.Directory(t), "lab", settings.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close() })
	engine, e := ingest.New(db, "lab", "")
	if e != nil {
		t.Fatal(e)
	}
	m := testfixture.Manifest()
	keys, secret := testfixture.Keyring()
	if e = engine.SetConfig(context.Background(), m, keys, settings.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	hash, e := settings.PasswordHash("diagnostic-test-only-password")
	if e != nil {
		t.Fatal(e)
	}
	a := settings.Auth{SchemaVersion: 1, Revision: "1", Users: []settings.User{{ID: "operator", Username: "operator", PasswordHash: hash, Enabled: true, Permissions: []string{"diagnostics.read"}}}}
	manager := access.New()
	if e = manager.Reload(a, "initial"); e != nil {
		t.Fatal(e)
	}
	app := New(settings.Server{InstanceID: "lab", Limits: settings.DefaultLimits()}, manager, engine, nil)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	app.Config.PublicOrigin = server.URL
	app.Config.AllowedHosts = []string{u.Host}
	return &httpLab{t: t, app: app, server: server, auth: a, manifest: m, keys: keys, secret: secret, db: db}
}

func TestRealHTTPIngestionCommitFailureReturns503WithoutPartialState(t *testing.T) {
	l := newHTTPLab(t)
	identity := ingest.SessionRequest{ProtocolMajor: 1, InstanceID: "lab", NodeID: "node", ManifestRevision: "1", AssignmentRevision: "1", Epoch: "1", BootID: uuid.NewString()}
	status, raw, _ := l.request("POST", "/observer/v1/sessions", identity, false, map[string]string{"Authorization": "Bearer " + l.secret})
	if status != 200 {
		t.Fatal("handshake", status)
	}
	var session ingest.SessionResponse
	if json.Unmarshal(raw, &session) != nil {
		t.Fatal("session response")
	}
	f := ingest.Frame{SessionRequest: identity, Sequence: "1", GeneratedAt: time.Now().UTC().Truncate(time.Millisecond), Observations: []model.Observation{}}
	for _, p := range l.manifest.Probes {
		f.Observations = append(f.Observations, model.New(p, identity.BootID, time.Now().UTC()))
	}
	if err := l.db.Close(); err != nil {
		t.Fatal(err)
	}
	status, _, _ = l.request("POST", "/observer/v1/reports", f, false, map[string]string{"Authorization": "Bearer " + session.Token})
	if status != 503 {
		t.Fatal("uncommitted HTTP acknowledgement", status)
	}
	if len(l.app.Engine.Read().Latest) != 0 {
		t.Fatal("partial state became visible")
	}
	status, _, _ = l.request("POST", "/observer/v1/sessions", identity, false, map[string]string{"Authorization": "Bearer " + l.secret})
	if status != 503 {
		t.Fatal("failed writer accepted handshake", status)
	}
}

func TestRealHTTPPayloadRateAndQuerySlotBounds(t *testing.T) {
	l := newHTTPLab(t)
	l.login()
	for i := 0; i < cap(l.app.querySlots); i++ {
		l.app.querySlots <- struct{}{}
	}
	status, _, _ := l.request("GET", base+"/overview", nil, true, nil)
	if status != 429 {
		t.Fatal("query slots unbounded", status)
	}
	for i := 0; i < cap(l.app.querySlots); i++ {
		<-l.app.querySlots
	}
	identity := ingest.SessionRequest{ProtocolMajor: 1, InstanceID: "lab", NodeID: "node", ManifestRevision: "1", AssignmentRevision: "1", Epoch: "1", BootID: uuid.NewString()}
	status, raw, _ := l.request("POST", "/observer/v1/sessions", identity, false, map[string]string{"Authorization": "Bearer " + l.secret})
	if status != 200 {
		t.Fatal(status)
	}
	var session ingest.SessionResponse
	_ = json.Unmarshal(raw, &session)
	headers := map[string]string{"Authorization": "Bearer " + session.Token}
	status, _, _ = l.request("POST", "/observer/v1/reports", map[string]string{"unknown": "strict-input-marker"}, false, headers)
	if status != 400 {
		t.Fatal("unknown input", status)
	}
	status, _, _ = l.request("POST", "/observer/v1/reports", strings.Repeat("x", (1<<20)+1), false, headers)
	if status != 413 {
		t.Fatal("oversized report", status)
	}
	status, _, _ = l.request("POST", "/observer/v1/reports", map[string]string{}, false, headers)
	if status != 429 {
		t.Fatal("report burst", status)
	}
	if len(l.app.Engine.Read().Latest) > 0 {
		t.Fatal("invalid input changed current state")
	}
	status, _, _ = l.request("GET", base+"/overview", nil, true, nil)
	if status != 200 {
		t.Fatal("report failures blocked query", status)
	}
}
func (l *httpLab) request(method, path string, body any, authorized bool, headers map[string]string) (int, []byte, http.Header) {
	l.t.Helper()
	var raw []byte
	if body != nil {
		var e error
		raw, e = json.Marshal(body)
		if e != nil {
			l.t.Fatal(e)
		}
	}
	r, e := http.NewRequest(method, l.server.URL+path, bytes.NewReader(raw))
	if e != nil {
		l.t.Fatal(e)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", l.server.URL)
	if authorized {
		for _, c := range l.cookies {
			r.AddCookie(c)
		}
		r.Header.Set("X-CSRF-Token", l.csrf)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, e := l.server.Client().Do(r)
	if e != nil {
		l.t.Fatal(e)
	}
	defer response.Body.Close()
	out, e := io.ReadAll(response.Body)
	if e != nil {
		l.t.Fatal(e)
	}
	for _, c := range response.Cookies() {
		if c.MaxAge >= 0 {
			l.cookies = append(l.cookies, c)
		}
	}
	return response.StatusCode, out, response.Header
}
func (l *httpLab) login() {
	l.t.Helper()
	status, raw, _ := l.request("GET", base+"/session/bootstrap", nil, false, nil)
	if status != 200 {
		l.t.Fatal(status)
	}
	var bootstrap struct {
		Data Bootstrap `json:"data"`
	}
	if json.Unmarshal(raw, &bootstrap) != nil {
		l.t.Fatal("bootstrap")
	}
	l.csrf = bootstrap.Data.CSRFToken
	status, raw, h := l.request("POST", base+"/session/login", map[string]string{"username": "operator", "password": "diagnostic-test-only-password"}, true, nil)
	if status != 200 {
		l.t.Fatalf("login: %d %s", status, raw)
	}
	var login struct {
		Data access.SessionView `json:"data"`
	}
	_ = json.Unmarshal(raw, &login)
	l.csrf = login.Data.CSRFToken
	if h.Get("Cache-Control") != "no-store" || !strings.Contains(h.Get("Set-Cookie"), "HttpOnly") || !strings.Contains(h.Get("Set-Cookie"), "SameSite=Strict") {
		l.t.Fatal("unsafe session headers")
	}
}
func TestPersonnelBoundariesOriginCSRFAndSafeErrors(t *testing.T) {
	l := newHTTPLab(t)
	for _, headers := range []map[string]string{nil, {"Cookie": "infrapro_session=business-administrator"}, {"Authorization": "Bearer " + l.keys.Keys[0].KeyID + "." + l.secret}} {
		s, raw, _ := l.request("GET", base+"/overview", nil, false, headers)
		if s != 401 || strings.Contains(string(raw), "Logical node") {
			t.Fatal("anonymous topology disclosure")
		}
	}
	l.login()
	for _, path := range []string{"/session/logout", "/session/activity"} {
		s, _, _ := l.request("POST", base+path, map[string]any{}, true, map[string]string{"Origin": "https://attacker.invalid"})
		if s != 403 {
			t.Fatal("cross-origin mutation")
		}
		s, _, _ = l.request("POST", base+path, map[string]any{}, true, map[string]string{"X-CSRF-Token": "wrong"})
		if s != 403 {
			t.Fatal("csrf mutation")
		}
	}
	s, raw, h := l.request("GET", base+"/overview", nil, true, nil)
	if s != 200 || h.Get("Cache-Control") != "no-store" {
		t.Fatal("protected query")
	}
	for _, secret := range []string{l.secret, l.auth.Users[0].PasswordHash, "diagnostic-test-only-password", "state.db", "command_line", "environment"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("sensitive output")
		}
	}
	s, raw, _ = l.request("GET", base+"/does-not-exist", nil, true, nil)
	if s != 404 || !json.Valid(raw) {
		t.Fatal("API SPA fallback")
	}
	s, _, _ = l.request("GET", "/assets/absent.js", nil, false, nil)
	if s != 404 {
		t.Fatal("missing asset fallback")
	}
	s, _, _ = l.request("POST", base+"/session/logout", map[string]any{}, true, nil)
	if s != 204 {
		t.Fatal("logout")
	}
	s, _, _ = l.request("GET", base+"/overview", nil, true, nil)
	if s != 401 {
		t.Fatal("logout did not revoke")
	}
}
func TestResponseRechecksAuthBeforeFirstByte(t *testing.T) {
	l := newHTTPLab(t)
	l.login()
	entered := make(chan struct{})
	release := make(chan struct{})
	l.app.beforeWrite = func() { close(entered); <-release }
	response := make(chan int, 1)
	go func() { s, _, _ := l.request("GET", base+"/overview", nil, true, nil); response <- s }()
	<-entered
	l.auth.Revision = "2"
	l.auth.Users[0].Enabled = false
	if e := l.app.Access.Reload(l.auth, "disabled"); e != nil {
		t.Fatal(e)
	}
	close(release)
	if s := <-response; s != 401 {
		t.Fatalf("stale response escaped %d", s)
	}
	l.app.beforeWrite = nil
	l.app.Access.Invalidate()
	s, _, _ := l.request("GET", base+"/overview", nil, true, nil)
	if s != 503 {
		t.Fatal("invalid auth not fail-closed")
	}
}
func TestPaginationSnapshotInvalidationAndBounds(t *testing.T) {
	l := newHTTPLab(t)
	l.manifest.Revision = "2"
	l.manifest.Components = append(l.manifest.Components, settings.Component{ID: "disabled", DisplayName: "Disabled", Kind: "tool", Expectation: "disabled"})
	if e := l.app.Engine.SetConfig(context.Background(), l.manifest, l.keys, settings.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	l.login()
	s, raw, _ := l.request("GET", base+"/components?limit=1", nil, true, nil)
	if s != 200 {
		t.Fatal(s)
	}
	var page struct {
		Data struct {
			Total int    `json:"total"`
			Next  string `json:"next_cursor"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &page)
	if page.Data.Total != 2 || page.Data.Next == "" {
		t.Fatal("pagination missing")
	}
	q := url.Values{"limit": {"1"}, "cursor": {page.Data.Next}}
	s, _, _ = l.request("GET", base+"/components?"+q.Encode(), nil, true, nil)
	if s != 200 {
		t.Fatal("next page")
	}
	l.manifest.Revision = "3"
	if e := l.app.Engine.SetConfig(context.Background(), l.manifest, l.keys, settings.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	s, _, _ = l.request("GET", base+"/components?"+q.Encode(), nil, true, nil)
	if s != 409 {
		t.Fatal("old cursor accepted")
	}
	for _, suffix := range []string{"?limit=101", "?limit=-1", "?unknown=sensitive", "?limit=1&limit=2"} {
		s, _, _ = l.request("GET", base+"/components"+suffix, nil, true, nil)
		if s != 400 {
			t.Fatalf("invalid filter accepted %s %d", suffix, s)
		}
	}
}
func TestPublicResponsesMatchIndependentOpenAPI(t *testing.T) {
	l := newHTTPLab(t)
	l.login()
	raw, e := os.ReadFile("../../../../../contracts/openapi/diagnostics.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var document any
	if json.Unmarshal(raw, &document) != nil {
		t.Fatal("contract")
	}
	compiler := jsonschema.NewCompiler()
	if e = compiler.AddResource("urn:diagnostics:contract", document); e != nil {
		t.Fatal(e)
	}
	for _, item := range []struct{ path, schema string }{{"/overview", "Overview"}, {"/nodes", "NodeViewPage"}, {"/nodes/node", "NodeDetail"}, {"/components", "ComponentViewPage"}, {"/components/api", "ComponentDetail"}, {"/replicas/api-1", "ReplicaView"}, {"/checks/report", "Check"}, {"/transitions", "TransitionPage"}, {"/self", "Self"}} {
		t.Run(item.schema, func(t *testing.T) {
			s, raw, _ := l.request("GET", base+item.path, nil, true, nil)
			if s != 200 {
				t.Fatalf("%d", s)
			}
			var envelope map[string]any
			if json.Unmarshal(raw, &envelope) != nil {
				t.Fatal("invalid response")
			}
			schema, e := compiler.Compile("urn:diagnostics:contract#/components/schemas/" + item.schema)
			if e != nil {
				t.Fatal(e)
			}
			if e = schema.Validate(envelope["data"]); e != nil {
				t.Fatal(e)
			}
		})
	}
	from := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	s, raw, _ := l.request("GET", base+"/metrics?"+url.Values{"target_kind": {"node"}, "target_id": {"node"}, "metric_key": {"cpu.utilization_ratio"}, "from": {from}, "to": {to}}.Encode(), nil, true, nil)
	if s != 200 {
		t.Fatalf("metrics: %d %s", s, raw)
	}
}
