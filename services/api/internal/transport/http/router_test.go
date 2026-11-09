package http_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/api/internal/modules/health"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func TestPublicHealthEndpoints(t *testing.T) {
	router := httptransport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, path := range []string{"/healthz", "/api/v1/health"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var body health.Response
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != "ok" || body.Service != "infrapro-api" || body.Version == "" || time.Since(body.Time) > time.Second || body.Time.After(time.Now()) {
				t.Fatalf("invalid health response: %+v", body)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Request-ID") == "" {
				t.Fatal("health must be uncached and traceable")
			}
		})
	}
}

func TestRoutingErrorsAreJSON(t *testing.T) {
	router := httptransport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodGet, "/api/v1/missing", "not_found", http.StatusNotFound},
		{http.MethodPost, "/api/v1/health", "method_not_allowed", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != tc.status || body.Error != tc.code {
			t.Fatalf("%s %s: status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}
