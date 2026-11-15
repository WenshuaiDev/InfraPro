package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAuditHTTPCommandsStrictQueryAndMetadataWithoutStorage(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:audit")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	_, e = env.Service.Bootstrap(ctx, "AuditHTTPOwner", "审计HTTP", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, token, _, e := env.Service.Login(ctx, "AuditHTTPOwner", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	p, e := env.Service.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	tenant, e := tenancy.New(env.Service.Access()).Create(ctx, env.Service.AccessActor(p), tenancy.CreateInput{Fields: tenancy.Fields{Name: "HTTP审计", Code: "audit-http"}, AdministratorID: p.Account.ID, RequestID: uuid.NewString(), Reason: "隔离HTTP验收"}, access.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path, body string, commands ...string) (int, map[string]any) {
		t.Helper()
		r, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		r.Header.Set("Origin", env.Config.TrustedOrigins[0])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		for _, v := range commands {
			r.Header.Add("X-Command-ID", v)
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("cacheable response")
		}
		data := map[string]any{}
		if len(raw) > 0 {
			if e = json.Unmarshal(raw, &data); e != nil {
				t.Fatal(e, string(raw))
			}
		}
		return response.StatusCode, data
	}
	_, out := call("GET", "/api/v1/auth/csrf", "")
	csrf = out["token"].(string)
	login, _ := json.Marshal(map[string]string{"username": "AuditHTTPOwner", "password": password})
	if status, v := call("POST", "/api/v1/auth/login", string(login)); status != 200 {
		t.Fatal(status, v)
	}
	for _, op := range access.CommandOperations() {
		policy, ok := access.PolicyFor(op)
		if !ok {
			t.Fatal(op)
		}
		path := strings.ReplaceAll(policy.Path, ":tenant_id", tenant.TenantID)
		for _, param := range []string{":id", ":role_id", ":member_id"} {
			path = strings.ReplaceAll(path, param, uuid.NewString())
		}
		for _, headers := range [][]string{nil, {uuid.NewString(), uuid.NewString()}} {
			status, v := call(policy.Method, path, `{}`, headers...)
			if status != 428 || v["code"] != "command_id_required" {
				t.Fatalf("%s missing/duplicate header: %d %v", op, status, v)
			}
		}
	}
	command := uuid.NewString()
	body := `{"name":"唯一HTTP角色","description":"","reason":"同命令一次效果"}`
	status, one := call("POST", "/api/v1/access/roles", body, command)
	if status != 200 {
		t.Fatal(status, one)
	}
	status, two := call("POST", "/api/v1/access/roles", body, command)
	if status != 200 || one["id"] != two["id"] {
		t.Fatal("HTTP replay", status, two)
	}
	status, v := call("POST", "/api/v1/access/roles", strings.Replace(body, "唯一HTTP角色", "不同载荷", 1), command)
	if status != 409 || v["code"] != "request_conflict" {
		t.Fatal(status, v)
	}
	for _, path := range []string{"/audit/events?limit=101", "/audit/events?limit=10&limit=20", "/audit/events?unknown=x", "/audit/events?actor_id=bad", "/audit/events?reference_id=bad", "/audit/exports?mode=all", "/audit/exports?state=made-up"} {
		status, v := call("GET", "/api/v1"+path, "")
		if status != 400 {
			t.Fatal(path, status, v)
		}
	}
	status, v = call("GET", "/api/v1/audit/capabilities", "")
	if status != 200 || v["storage_available"] != false {
		t.Fatal("metadata boot unexpectedly needs S3", status, v)
	}
	for _, body := range []string{`{"reason":"a","reason":"b"}`, `{"filter":{"q":"a","Q":"b"}}`, `{"filter":[],"reason":"a"}`, `{"unexpected":true}`, `{}` + `{}`} {
		status, _ = call("POST", "/api/v1/audit/exports", body, uuid.NewString())
		if status != 400 {
			t.Fatal("invalid audit JSON accepted", status)
		}
	}
	status, v = call("GET", "/api/v1/audit/events", "")
	if status != 200 || !bytes.Contains(mustJSON(v), []byte("items")) {
		t.Fatal(status, v)
	}
	t.Log("all 13 command-header routes reject missing/duplicate IDs with 428; HTTP replay/conflict, strict filters/JSON and S3-free metadata boot passed")
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
