package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func TestHTTPSOriginAndCSRFBehindHTTPProxy(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity with isolated PostgreSQL and Redis")
	}
	env, err := testsupport.New(context.Background(), logger)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	origin := "https://10.23.52.152:18443"
	cfg := config.Config{CSRFKey: []byte("0123456789abcdef0123456789abcdef"), CookieSecure: true, TrustedOrigins: []string{origin}, TrustedProxies: []string{"127.0.0.1"}}
	h := httptransport.NewRouter(logger, env.Service, cfg)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://10.23.52.152:18443/api/v1/auth/csrf", nil))
	var csrf struct{ Token string }
	if err := json.Unmarshal(w.Body.Bytes(), &csrf); err != nil || csrf.Token == "" {
		t.Fatal("CSRF bootstrap", err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Domain != "" {
		t.Fatal("HTTPS CSRF cookie boundary")
	}
	for _, tc := range []struct {
		origin, token, forwarded string
		status                   int
		code                     string
	}{
		{origin, csrf.Token, "http", 401, "unauthenticated"}, // Valid CSRF reaches the router over plaintext loopback.
		{origin, "", "https", 403, "csrf_invalid"},
		{origin, "forged", "https", 403, "csrf_invalid"},
		{"http://10.23.52.152:18443", csrf.Token, "https", 403, "origin_rejected"},
		{"https://10.23.52.152:18444", csrf.Token, "https", 403, "origin_rejected"},
		{"https://attacker.example", csrf.Token, "https", 403, "origin_rejected"},
		{"", csrf.Token, "https", 403, "origin_rejected"},
	} {
		r := httptest.NewRequest("POST", "http://10.23.52.152:18443/api/v1/not-a-route", nil)
		r.RemoteAddr = "192.0.2.1:10000"
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-CSRF-Token", tc.token)
		r.Header.Set("X-Forwarded-Proto", tc.forwarded)
		r.Header.Set("X-Forwarded-Host", "attacker.example")
		r.AddCookie(cookies[0])
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("origin %q, token present %v: %d %s", tc.origin, tc.token != "", w.Code, w.Body)
		}
	}
}
