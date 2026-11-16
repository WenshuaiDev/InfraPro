package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParsingHTTPStrictBoundariesAndDisabledHistory(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:parsing")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if _, e = env.Service.Bootstrap(ctx, "ParsingOwner", "任务测试", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path string, body any, write bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Origin", env.Config.TrustedOrigins[0])
		req.Header.Set("Content-Type", "application/json")
		if write {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		r, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		if !strings.Contains(r.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("task response cacheable")
		}
		out := map[string]any{}
		if r.StatusCode != 204 {
			if e = json.NewDecoder(r.Body).Decode(&out); e != nil {
				t.Fatal(e)
			}
		}
		return r.StatusCode, out
	}
	if code, _ := call("GET", "/platform/parsing/methods", nil, false); code != 401 {
		t.Fatal(code)
	}
	_, v := call("GET", "/auth/csrf", nil, false)
	csrf = v["token"].(string)
	if code, v := call("POST", "/auth/login", map[string]string{"username": "ParsingOwner", "password": password}, true); code != 200 {
		t.Fatal(code, v)
	}
	for _, path := range []string{"/platform/parsing/methods", "/platform/parsing/runs", "/platform/parsing/samples"} {
		if code, v := call("GET", path, nil, false); code != 200 {
			t.Fatal(path, code, v)
		}
	}
	id := uuid.NewString()
	for _, input := range []map[string]any{{"request_id": id, "reason": "test", "service_url": "http://example.invalid"}, {"request_id": id, "reason": "test", "code": "arbitrary"}, {"request_id": id, "reason": "test", "actor_id": id}} {
		if code, _ := call("POST", "/platform/parsing/catalog-syncs", input, true); code != 400 {
			t.Fatal("unexpected fields allowed", code)
		}
	}
	if code, _ := call("POST", "/platform/parsing/catalog-syncs", map[string]string{"request_id": id, "reason": "test"}, false); code != 403 {
		t.Fatal("CSRF bypass", code)
	}
	for _, path := range []string{"/platform/parsing/methods?service_url=x", "/platform/parsing/runs?limit=100000", "/platform/parsing/runs/" + id + "/executions/1/result?offset=-1", "/platform/parsing/samples/" + id + "/content?representation=false"} {
		if code, _ := call("GET", path, nil, false); code < 400 || code >= 500 {
			t.Fatal("query boundary", path, code)
		}
	}
	for _, path := range []string{"/platform/parsing/methods", "/platform/parsing/code", "/platform/parsing/service-url"} {
		if code, _ := call("POST", path, map[string]string{"code": "arbitrary"}, true); code < 400 || code >= 500 {
			t.Fatal("code registration route", code)
		}
	}
	t.Log("current session, CSRF, strict fields, bounded query, absent code/URL mutation routes and disabled-service history verified")
}
