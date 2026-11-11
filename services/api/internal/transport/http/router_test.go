package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

const password = "Real browser 河流 secret 592"

func TestPublicProbesAndOrigin(t *testing.T) {
	cfg := config.Config{CSRFKey: []byte("0123456789abcdef0123456789abcdef"), TrustedOrigins: []string{"http://127.0.0.1:15173"}}
	router := httptransport.NewRouter(logger, nil, cfg)
	for _, probe := range []struct {
		path   string
		status int
	}{{"/healthz", 200}, {"/readyz", 503}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", probe.path, nil))
		if w.Code != probe.status {
			t.Fatalf("probe %s: %d %s", probe.path, w.Code, w.Body.String())
		}
		if probe.status == 200 && w.Body.String() != `{"status":"ok"}` {
			t.Fatalf("unexpected liveness response: %s", w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("headers missing")
		}
	}
	for _, origin := range []string{"", "http://attacker.test", "http://127.0.0.1:15173.evil", "https://127.0.0.1:15173"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.Header.Set("Origin", origin)
		router.ServeHTTP(w, r)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "origin_rejected") {
			t.Fatal("bad origin allowed")
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	r.Header.Set("Origin", cfg.TrustedOrigins[0])
	router.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "csrf_invalid") {
		t.Fatal("missing CSRF allowed")
	}
}
func TestHTTPAccountBoundary(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if _, e = env.Service.Bootstrap(ctx, "Alice", "甲", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	request := func(method, path string, body any, token bool) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", env.Config.TrustedOrigins[0])
		if token {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var data map[string]any
		if response.StatusCode != 204 {
			if err = json.NewDecoder(response.Body).Decode(&data); err != nil {
				t.Fatal(err)
			}
		}
		if response.Header.Get("X-Request-ID") == "" {
			t.Fatal("no request ID")
		}
		return response.StatusCode, data
	}
	code, _ := request("GET", "/api/v1/auth/me", nil, false)
	if code != 401 {
		t.Fatalf("unprotected me: %d", code)
	}
	code, v := request("GET", "/api/v1/auth/csrf", nil, false)
	if code != 200 {
		t.Fatal(code, v)
	}
	csrf = v["token"].(string)
	code, v = request("POST", "/api/v1/auth/login", map[string]string{"username": "Alice", "password": password}, true)
	if code != 200 {
		t.Fatal(code, v)
	}
	code, _ = request("GET", "/api/v1/health", nil, false)
	if code != 404 {
		t.Fatal("removed demonstration endpoint is still available", code)
	}
	// Ownership cannot be selected by an extra payload field, and cookie attributes work on HTTP.
	code, _ = request("PATCH", "/api/v1/account/profile", map[string]string{"display_name": "new", "account_id": "other"}, true)
	if code != 400 {
		t.Fatal("extra identity field allowed", code)
	}
	code, v = request("POST", "/api/v1/invitations", map[string]string{"note": "test"}, true)
	if code != 200 {
		t.Fatal(code, v)
	}
	link := v["url"].(string)
	token := strings.Split(link, "#token=")[1]
	code, _ = request("POST", "/api/v1/public/invitations/accept", map[string]string{"token": token, "username": "Bob", "password": password}, true)
	if code != 409 {
		t.Fatal("signed-in user may accept invite", code)
	}
	code, _ = request("POST", "/api/v1/auth/logout", nil, true)
	if code != 204 {
		t.Fatal(code)
	}
	code, _ = request("POST", "/api/v1/public/invitations/accept", map[string]string{"token": token, "username": "Bob", "password": password}, true)
	if code != 204 {
		t.Fatal(code)
	}
	code, _ = request("POST", "/api/v1/auth/login", map[string]string{"username": "Bob", "password": password}, true)
	if code != 200 {
		t.Fatal(code)
	}
	var aliceSession string
	if e = env.Pool.QueryRow(ctx, "SELECT id::text FROM account_sessions WHERE account_id=(SELECT id FROM accounts WHERE username='Alice') LIMIT 1").Scan(&aliceSession); e != nil {
		t.Fatal(e)
	}
	code, _ = request("DELETE", "/api/v1/account/sessions/"+aliceSession, nil, true)
	if code != 404 {
		t.Fatal("cross-account session allowed", code)
	}
	code, v = request("GET", "/api/v1/invitations", nil, false)
	if code != 403 || v["code"] != "permission_denied" {
		t.Fatal("member invitation permission boundary", code, v)
	}
	code, v = request("GET", "/api/v1/account/security-events", nil, false)
	if code != 200 {
		t.Fatal(code)
	}
	for _, entry := range v["items"].([]any) {
		event := entry.(map[string]any)
		if event["event_type"] == "bootstrap" {
			t.Fatal("security events leaked")
		}
	}
	code, _ = request("GET", "/api/v1/unknown", nil, false)
	if code != 404 {
		t.Fatal("unknown API should remain JSON", code)
	}
}

// TestE2EServer is a test-only process: it seeds isolated test data, serves the real HTTP handler,
// and drops only its own schema and Redis namespace on SIGTERM.
func TestE2EServer(t *testing.T) {
	if os.Getenv("INFRAPRO_E2E_SERVER") != "1" {
		t.Skip("test-only browser fixture")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if value := os.Getenv("INFRAPRO_E2E_CONSOLE_URL"); value != "" {
		env.Config.PublicConsoleURL = value
		env.Config.TrustedOrigins = strings.Split(os.Getenv("INFRAPRO_E2E_ORIGINS"), ",")
	}
	env.Service, e = identity.NewService(env.Pool, env.Redis, env.Config, logger)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = env.Service.Bootstrap(ctx, "e2e_owner", "浏览器测试账号", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	if p := os.Getenv("INFRAPRO_E2E_FIXTURE_FILE"); p != "" {
		fixture := map[string]string{}
		_, ownerToken, _, err := env.Service.Login(ctx, "e2e_owner", password, identity.Meta{})
		if err != nil {
			t.Fatal(err)
		}
		owner, err := env.Service.Authenticate(ctx, ownerToken)
		if err != nil {
			t.Fatal(err)
		}
		for _, project := range []string{"desktop", "mobile"} {
			username := "e2e_reset_" + project
			link, err := env.Service.CreateInvitation(ctx, owner, "", "reset fixture", identity.Meta{})
			if err != nil {
				t.Fatal(err)
			}
			token := strings.Split(link.URL, "#token=")[1]
			if err = env.Service.AcceptInvitation(ctx, token, username, "", password, identity.Meta{}); err != nil {
				t.Fatal(err)
			}
			reset, err := env.Service.ResetLink(ctx, username, identity.Meta{Reason: "browser reset verification"})
			if err != nil {
				t.Fatal(err)
			}
			fixture["reset_"+project] = reset.URL
		}
		b, _ := json.Marshal(fixture)
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		defer os.Remove(p)
	}
	address := os.Getenv("INFRAPRO_HTTP_ADDR")
	if address == "" {
		address = "127.0.0.1:18080"
	}
	server := &http.Server{Addr: address, Handler: httptransport.NewRouter(logger, env.Service, env.Config), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	jobs, stopJobs := context.WithCancel(ctx)
	defer stopJobs()
	go env.Service.RunJobs(jobs)
	fmt.Println("isolated account E2E server ready")
	select {
	case e := <-done:
		t.Fatal(e)
	case <-ctx.Done():
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(closeCtx)
	}
}

// Route registration itself is dependency-free; construction rejects every unclassified route.
func TestRoutePolicyCoverage(t *testing.T) {
	cfg := config.Config{CSRFKey: []byte("0123456789abcdef0123456789abcdef"), TrustedOrigins: []string{"http://127.0.0.1:15173"}}
	_ = httptransport.NewRouter(logger, &identity.Service{}, cfg)
}
