package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func TestTenantHTTPPartialPatchReauthAndCSRF(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity")
	}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	env, e := testsupport.New(ctx, logger)
	must(e)
	defer env.Close()
	admin, e := env.Service.Bootstrap(ctx, "Owner", "平台", password, identity.Meta{})
	must(e)
	_, token, _, e := env.Service.Login(ctx, "Owner", password, identity.Meta{})
	must(e)
	p, e := env.Service.Authenticate(ctx, token)
	must(e)
	link, e := env.Service.CreateInvitation(ctx, p, "", "", identity.Meta{})
	must(e)
	u, _ := url.Parse(link.URL)
	q, _ := url.ParseQuery(u.Fragment)
	must(env.Service.AcceptInvitation(ctx, q.Get("token"), "Member", "接任", password, identity.Meta{}))
	var member string
	must(env.Pool.QueryRow(ctx, "SELECT id::text FROM accounts WHERE username='Member'").Scan(&member))
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	request := func(method, path string, body any, origin, verification string) (int, map[string]any) {
		t.Helper()
		data, _ := json.Marshal(body)
		r, _ := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", verification)
		res, e := client.Do(r)
		must(e)
		defer res.Body.Close()
		out := map[string]any{}
		if res.StatusCode != 204 {
			must(json.NewDecoder(res.Body).Decode(&out))
		}
		return res.StatusCode, out
	}
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		status, v := request(method, path, body, env.Config.TrustedOrigins[0], csrf)
		if status != want {
			t.Fatalf("%s %s status %d want %d: %v", method, path, status, want, v)
		}
		return v
	}
	call("GET", "/platform/tenants", nil, 401)
	csrf = call("GET", "/auth/csrf", nil, 200)["token"].(string)
	call("POST", "/auth/login", map[string]string{"username": "Owner", "password": password}, 200)
	var requestID string
	must(env.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&requestID))
	input := map[string]any{"name": "初始", "code": "http-partial", "description": "保留说明", "administrator_account_id": admin.ID, "client_request_id": requestID, "reason": "创建"}
	result := call("POST", "/platform/tenants", input, 201)
	id := result["tenant_id"].(string)
	// PATCH is partial, including explicit clearing, and cannot silently clear omitted fields.
	call("PATCH", "/tenants/"+id+"/profile", map[string]string{"expected_version": "1", "name": "部分修改", "reason": "仅名称"}, 200)
	detail := call("GET", "/tenants/"+id+"/profile", nil, 200)
	if detail["code"] != "http-partial" || detail["description"] != "保留说明" {
		t.Fatal(detail)
	}
	call("PATCH", "/tenants/"+id+"/profile", map[string]string{"expected_version": "2", "reason": "无字段"}, 400)
	call("PATCH", "/tenants/"+id+"/profile", map[string]string{"expected_version": "2", "description": "", "reason": "清空说明"}, 200)
	call("PATCH", "/tenants/"+id+"/profile", map[string]string{"expected_version": "3", "description": "", "reason": "无实质变化"}, 200)
	detail = call("GET", "/tenants/"+id+"/profile", nil, 200)
	if detail["version"] != "3" || detail["description"] != "" {
		t.Fatal(detail)
	}
	transfer := map[string]string{"expected_version": "3", "new_administrator_account_id": member, "reason": "交接"}
	for _, h := range []struct{ origin, csrf, code string }{{"http://attacker.invalid", csrf, "origin_rejected"}, {env.Config.TrustedOrigins[0], "", "csrf_invalid"}} {
		status, v := request("POST", "/platform/tenants/"+id+"/administrator-transfer", transfer, h.origin, h.csrf)
		if status != 403 || v["code"] != h.code {
			t.Fatal(status, v)
		}
	}
	_, e = env.Pool.Exec(ctx, "UPDATE account_sessions SET reauthenticated_at=now()-interval '6 minutes' WHERE account_id=$1", admin.ID)
	must(e)
	denied := call("POST", "/platform/tenants/"+id+"/administrator-transfer", transfer, 403)
	if denied["code"] != "reauthentication_required" {
		t.Fatal(denied)
	}
	call("POST", "/auth/reauthenticate", map[string]string{"password": password}, 204)
	detail = call("GET", "/platform/tenants/"+id, nil, 200)
	if detail["version"] != "3" {
		t.Fatal("reauth replayed operation")
	}
	call("POST", "/platform/tenants/"+id+"/administrator-transfer", transfer, 200)
	noMember := call("GET", "/tenants/"+id+"/profile", nil, 404)
	unknown := call("GET", "/tenants/00000000-0000-0000-0000-000000000000/profile", nil, 404)
	if noMember["message"] != unknown["message"] || noMember["code"] != unknown["code"] {
		t.Fatal("existence leaked")
	}
	call("GET", "/tenants/"+id+"/audit-events", nil, 404)
}
