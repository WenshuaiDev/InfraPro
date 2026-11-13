package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
)

func TestDerivedDefinitionHTTPContract(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated test database")
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
	a, e := env.Service.Bootstrap(ctx, "Owner", "平台", password, identity.Meta{})
	must(e)
	_, token, _, e := env.Service.Login(ctx, "Owner", password, identity.Meta{})
	must(e)
	p, e := env.Service.Authenticate(ctx, token)
	must(e)
	tenant, e := tenancy.New(env.Service.Access()).Create(ctx, env.Service.AccessActor(p), tenancy.CreateInput{Fields: tenancy.Fields{Name: "HTTP 派生", Code: "http-derived"}, AdministratorID: a.ID, RequestID: uuid.NewString(), Reason: "HTTP 验证"}, access.Meta{})
	must(e)
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, e := cookiejar.New(nil)
	must(e)
	client := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path string, body any, verification string, want int) map[string]any {
		t.Helper()
		var b []byte
		if raw, ok := body.(json.RawMessage); ok {
			b = raw
		} else {
			b, e = json.Marshal(body)
			must(e)
		}
		r, e := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(b))
		must(e)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", env.Config.TrustedOrigins[0])
		r.Header.Set("X-CSRF-Token", verification)
		res, e := client.Do(r)
		must(e)
		defer res.Body.Close()
		out := map[string]any{}
		must(json.NewDecoder(res.Body).Decode(&out))
		if res.StatusCode != want {
			t.Fatalf("%s %s got %d want %d: %v", method, path, res.StatusCode, want, out)
		}
		return out
	}
	base := "/tenants/" + tenant.TenantID + "/monitoring/derived"
	call("GET", base+"/templates", nil, csrf, 401)
	csrf = call("GET", "/auth/csrf", nil, csrf, 200)["token"].(string)
	call("POST", "/auth/login", map[string]string{"username": "Owner", "password": password}, csrf, 200)
	call("GET", base+"/templates?unknown=1", nil, csrf, 400)
	call("GET", base+"/metric-definitions?limit=2&limit=3", nil, csrf, 400)
	call("POST", base+"/metric-definitions", nil, "", 403)
	templates := call("GET", base+"/templates", nil, csrf, 200)
	if len(templates["items"].([]any)) != 8 {
		t.Fatal(templates)
	}
	rule := json.RawMessage(`{"schema_version":"derived-rule/1","inputs":[{"key":"A","shape":"numeric","source_kinds":["direct"],"quantity_spec":{"base_power":0,"time_power":0,"quantity_mode":"ratio","level_scale":"ratio","dimensionless_kind":"ratio"},"direction":"not_applicable","reference":"not_applicable","time_semantics":{"kind":"instant"}}],"clock":{"kind":"primary_input","input":"A"},"nodes":[{"id":"absolute","op":"abs","inputs":["input:A"],"params":{}}],"outputs":[{"key":"value","node":"absolute","semantic_key":"absolute","numeric_contract":{"scale":18,"rounding":"half_even"}}],"policies":{"config_boundary":"break","allowed_lateness_ns":"0"}}`)
	input := map[string]any{"request_id": uuid.NewString(), "code": "absolute", "name": "绝对值", "description": "HTTP 规则", "rule": rule}
	input["tenant_id"] = uuid.NewString()
	call("POST", base+"/metric-definitions", input, csrf, 400)
	delete(input, "tenant_id")
	input["rule"] = json.RawMessage(strings.Replace(string(rule), `"shape":"numeric"`, `"Shape":"numeric"`, 1))
	bad := call("POST", base+"/metric-definitions", input, csrf, 400)
	if bad["code"] != "unknown_field" {
		t.Fatal("unknown rule property lost its issue", bad)
	}
	input["rule"] = json.RawMessage(strings.Replace(string(rule), `"op":"abs"`, `"op":"shell"`, 1))
	bad = call("POST", base+"/metric-definitions", input, csrf, 422)
	if len(bad["issues"].([]any)) == 0 {
		t.Fatal(bad)
	}
	input["rule"] = rule
	d := call("POST", base+"/metric-definitions", input, csrf, 201)["definition"].(map[string]any)
	definition := d["id"].(string)
	validation := call("POST", base+"/metric-definitions/"+definition+"/validate", map[string]string{"draft_revision": d["revision"].(string)}, csrf, 200)
	publish := map[string]any{"request_id": uuid.NewString(), "expected_revision": d["revision"], "draft_revision": d["revision"], "validation_hash": validation["validation_hash"]}
	v := call("POST", base+"/metric-definitions/"+definition+"/publish", publish, csrf, 201)["version"].(map[string]any)
	if v["version_no"] != "1" || v["state_revision"] != "1" || len(v["content_hash"].(string)) != 64 {
		t.Fatal("public precision or version contract", v)
	}
	replay := call("POST", base+"/metric-definitions/"+definition+"/publish", publish, csrf, 201)
	if replay["replayed"] != true {
		t.Fatal("HTTP replay missing", replay)
	}
	call("GET", base+"/metric-versions/"+v["id"].(string), nil, csrf, 200)
	call("GET", base+"/metric-definitions/not-a-uuid", nil, csrf, 404)
	call("POST", base+"/metric-definitions", json.RawMessage(`{"request_id":"a","request_id":"b"}`), csrf, 400)
	call("POST", base+"/metric-definitions", json.RawMessage(`{"name":"`+strings.Repeat("x", 256*1024)+`"}`), csrf, 413)
}
