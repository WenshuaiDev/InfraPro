package displayhttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
