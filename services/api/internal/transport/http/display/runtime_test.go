package displayhttp

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPDatabaseFailureIsDependencyUnavailable(t *testing.T) {
	pool, e := pgxpool.New(context.Background(), "postgres://127.0.0.1:1/unavailable_test?connect_timeout=1")
	if e != nil {
		t.Fatal(e)
	}
	pool.Close()
	_, e = pool.Query(context.Background(), "SELECT 1")
	if e == nil {
		t.Fatal("closed pool accepted read")
	}
	problem := PublicError(e)
	if problem.Code != "dependency_unavailable" || problem.Status != 503 {
		t.Fatal("database dependency failure was not explicit", problem)
	}
}

func TestHTTPRuntimeBoundaries(t *testing.T) {
	s := display.New(nil)
	h := RuntimeHandler(s)
	for _, test := range []struct {
		method, path, host, origin string
		want                       int
	}{
		{"GET", "/healthz", "127.0.0.1:5180", "", 200},
		{"GET", "/runtime-config", "127.0.0.1:5180", "", 200},
		{"GET", "/runtime-config", "other.example", "", 403},
		{"POST", "/runtime/exchange", "127.0.0.1:5180", "http://other.example", 403},
	} {
		r := httptest.NewRequest(test.method, "http://"+test.host+test.path, strings.NewReader(`{}`))
		r.Header.Set("Origin", test.origin)
		r.Header.Set("X-Display-Protocol", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("%s %s: %d %s", test.method, test.path, w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	setRuntimeCookie(w, s.Options(), display.Session{ID: "runtime-id"}, "test-token")
	c := w.Result().Cookies()[0]
	if c.Secure || !c.HttpOnly || c.Domain != "" || c.Path != "/runtime/runtime-id/" || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("HTTP cookie boundary: %#v", c)
	}
}

func TestHTTPSRuntimeCookiesAndBoundaries(t *testing.T) {
	opts := display.DefaultOptions()
	opts.RuntimeOrigin = "https://10.23.52.152:18444"
	opts.AssetOrigin = "https://10.23.52.152:18445"
	opts.Secure = true
	s := display.New(nil).WithOptions(opts)
	w := httptest.NewRecorder()
	setRuntimeCookie(w, opts, display.Session{ID: "secure-session"}, "runtime-token")
	setTerminalCookie(w, opts, "terminal-token")
	for _, cookie := range w.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatal("HTTPS runtime/terminal cookie boundary")
		}
	}
	h := RuntimeHandler(s)
	for _, origin := range []string{"http://10.23.52.152:18444", "https://10.23.52.152:18443", "https://attacker.example"} {
		r := httptest.NewRequest("POST", "http://10.23.52.152:18444/runtime/exchange", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Display-Protocol", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("untrusted runtime origin accepted")
		}
	}
}
