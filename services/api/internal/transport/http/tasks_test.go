package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTaskHTTPAuthenticationCSRFStrictInputAndIdle(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:tasks")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if _, e = env.Service.Bootstrap(ctx, "TaskOwner", "任务测试", password, identity.Meta{}); e != nil {
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
	if code, _ := call("GET", "/tasks", nil, false); code != 401 {
		t.Fatal(code)
	}
	_, v := call("GET", "/auth/csrf", nil, false)
	csrf = v["token"].(string)
	if code, v := call("POST", "/auth/login", map[string]string{"username": "TaskOwner", "password": password}, true); code != 200 {
		t.Fatal(code, v)
	}
	var before, after time.Time
	if e = env.Pool.QueryRow(ctx, `SELECT last_activity_at FROM account_sessions ORDER BY created_at DESC LIMIT 1`).Scan(&before); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/tasks", "/tasks/summary", "/tasks/catalog", "/tasks/operations"} {
		if code, v := call("GET", path, nil, false); code != 200 {
			t.Fatal(path, code, v)
		}
	}
	if e = env.Pool.QueryRow(ctx, `SELECT last_activity_at FROM account_sessions ORDER BY created_at DESC LIMIT 1`).Scan(&after); e != nil || !after.Equal(before) {
		t.Fatal("task polling extended idle", e)
	}
	for _, path := range []string{"/tasks?account_id=other", "/tasks?result_url=https://example.invalid", "/tasks/summary?limit=5000", "/tasks/resolve?module=arbitrary&kind=job&source_id=11111111-1111-4111-8111-111111111111"} {
		if code, _ := call("GET", path, nil, false); code < 400 || code >= 500 {
			t.Fatalf("invalid task request %s: %d", path, code)
		}
	}
	id := "11111111-1111-4111-8111-111111111111"
	path := "/tasks/" + id + "/commands"
	cmd := map[string]any{"request_id": id, "action": "cancel", "expected_generation": "1", "expected_control_revision": "1", "reason": "停止本人任务"}
	if code, _ := call("POST", path, cmd, false); code != 403 {
		t.Fatal("CSRF bypass", code)
	}
	cmd["actor_id"] = id
	if code, v := call("POST", path, cmd, true); code != 400 {
		t.Fatal("identity injection", code, v)
	}
	delete(cmd, "actor_id")
	if code, _ := call("POST", path, cmd, true); code != 404 {
		t.Fatal("unknown source", code)
	}
	for _, suffix := range []string{"", "/executions", "/events?generation=1", "/target?generation=1"} {
		if code, _ := call("GET", "/tasks/"+id+suffix, nil, false); code != 404 {
			t.Fatal("existence disclosure", suffix, code)
		}
	}
}
