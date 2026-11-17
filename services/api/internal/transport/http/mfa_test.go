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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

type mfaHTTPClient struct {
	t                  *testing.T
	client             *http.Client
	base, origin, csrf string
}

func newMFAHTTPClient(t *testing.T, server *httptest.Server, origin string) *mfaHTTPClient {
	jar, _ := cookiejar.New(nil)
	baseClient := *server.Client()
	client := &baseClient
	client.Jar = jar
	a := &mfaHTTPClient{t: t, client: client, base: server.URL, origin: origin}
	_, body := a.call("GET", "/auth/csrf", nil, 200)
	a.csrf = body["token"].(string)
	return a
}
func (a *mfaHTTPClient) call(method, path string, input any, status int) (*http.Response, map[string]any) {
	a.t.Helper()
	b, _ := json.Marshal(input)
	r, _ := http.NewRequest(method, a.base+"/api/v1"+path, bytes.NewReader(b))
	r.Header.Set("Origin", a.origin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", a.csrf)
	r.Header.Set("X-Command-ID", uuid.NewString())
	response, e := a.client.Do(r)
	if e != nil {
		a.t.Fatal("HTTP transport failure")
	}
	defer response.Body.Close()
	var data map[string]any
	if response.StatusCode != 204 {
		if e = json.NewDecoder(response.Body).Decode(&data); e != nil {
			a.t.Fatal("invalid JSON response", response.StatusCode)
		}
	}
	if response.StatusCode != status {
		a.t.Fatalf("%s %s: status %d, expected %d; code %v", method, path, response.StatusCode, status, data["code"])
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		a.t.Fatal("sensitive response cacheable")
	}
	return response, data
}
func TestMFAHTTPBoundary(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:mfa")
	}
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "HTTPS"}[secure], func(t *testing.T) {
			ctx := context.Background()
			env, e := testsupport.New(ctx, logger)
			if e != nil {
				t.Fatal(e)
			}
			defer env.Close()
			_, e = env.Service.Bootstrap(ctx, "MfaAdmin", "MFA 合成管理员", password, identity.Meta{})
			if e != nil {
				t.Fatal(e)
			}
			env.Config.CookieSecure = secure
			if secure {
				env.Config.PublicConsoleURL = "https://127.0.0.1:15173/"
				env.Config.TrustedOrigins = []string{"https://127.0.0.1:15173"}
			}
			handler := httptransport.NewRouter(logger, env.Service, env.Config)
			var server *httptest.Server
			if secure {
				server = httptest.NewTLSServer(handler)
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			admin := newMFAHTTPClient(t, server, env.Config.TrustedOrigins[0])
			response, body := admin.call("POST", "/auth/login", map[string]string{"username": "MfaAdmin", "password": password}, 200)
			if body["status"] != "authenticated" {
				t.Fatal("login result not discriminated")
			}
			for _, c := range response.Cookies() {
				if c.Name == "infrapro_session" && (!c.HttpOnly || c.Secure != secure || c.Path != "/" || c.SameSite != http.SameSiteLaxMode) {
					t.Fatal("session cookie flags")
				}
			}
			admin.call("POST", "/auth/login", map[string]string{"username": "MfaAdmin", "password": password}, 409)
			_, link := admin.call("POST", "/invitations", map[string]string{"note": "MFA HTTP synthetic"}, 200)
			parsed, _ := url.Parse(link["url"].(string))
			fragment, _ := url.ParseQuery(parsed.Fragment)
			user := newMFAHTTPClient(t, server, env.Config.TrustedOrigins[0])
			user.call("POST", "/public/invitations/accept", map[string]string{"token": fragment.Get("token"), "username": "MfaUser", "password": password}, 204)
			user.call("POST", "/auth/login", map[string]string{"username": "MfaUser", "password": password}, 200)
			user.call("GET", "/platform/security-policy", nil, 403)
			user.call("PUT", "/platform/security-policy", map[string]any{"require_mfa": true, "expected_version": "1", "reason": "denied"}, 403)
			user.call("GET", "/account/mfa", nil, 200)
			_, setup := user.call("POST", "/account/mfa/enrollment/start", map[string]string{"mode": "enable", "expected_factor_version": "1"}, 200)
			user.call("POST", "/account/mfa/enrollment/confirm", map[string]string{"enrollment_id": setup["enrollment_id"].(string), "new_totp_code": "invalid", "mfa_verified": "true"}, 400)
			user.call("POST", "/account/mfa/enrollment/confirm", map[string]string{"enrollment_id": setup["enrollment_id"].(string), "new_totp_code": "invalid"}, 400)
			code, e := totp.GenerateCode(setup["secret"].(string), time.Now().Add(-30*time.Second))
			if e != nil {
				t.Fatal("OTP fixture")
			}
			_, enabled := user.call("POST", "/account/mfa/enrollment/confirm", map[string]string{"enrollment_id": setup["enrollment_id"].(string), "new_totp_code": code}, 200)
			if enabled["state"] != "enabled" || len(enabled["recovery_codes"].([]any)) != 10 {
				t.Fatal("incomplete enrollment")
			}
			user.call("POST", "/auth/logout", nil, 204)
			response, challenge := user.call("POST", "/auth/login", map[string]string{"username": "MfaUser", "password": password}, 202)
			for _, c := range response.Cookies() {
				if c.Name == "infrapro_session" && c.Value != "" {
					t.Fatal("password issued session before MFA")
				}
				if c.Name == "infrapro_auth_challenge" && (!c.HttpOnly || c.Secure != secure || c.Domain != "" || c.Path != "/api/v1/auth" || c.SameSite != http.SameSiteLaxMode || c.MaxAge > 300) {
					t.Fatal("challenge cookie flags")
				}
			}
			for _, path := range []string{"/auth/me", "/account/mfa", "/account/tenants", "/me/notifications", "/me/tasks", "/tenants/" + uuid.NewString() + "/context", "/tenants/" + uuid.NewString() + "/files", "/platform/security-policy", "/access/accounts"} {
				user.call("GET", path, nil, 401)
			}
			user.call("POST", "/auth/mfa/verify", map[string]string{"challenge_id": uuid.NewString(), "method": "totp", "code": "123456"}, 401)
			user.call("POST", "/auth/mfa/cancel", map[string]string{"challenge_id": uuid.NewString()}, 401)
			user.call("POST", "/auth/mfa/verify", map[string]string{"method": "totp", "code": "123456"}, 401)
			user.call("POST", "/auth/mfa/enrollment/start", map[string]string{"challenge_id": challenge["challenge_id"].(string)}, 401)
			_, resumed := user.call("GET", "/auth/mfa/challenge", nil, 200)
			if resumed["challenge_id"] != challenge["challenge_id"] {
				t.Fatal("challenge changed on refresh")
			}
			_, verified := user.call("POST", "/auth/mfa/verify", map[string]string{"challenge_id": challenge["challenge_id"].(string), "method": "recovery_code", "code": enabled["recovery_codes"].([]any)[0].(string)}, 200)
			if verified["status"] != "authenticated" {
				t.Fatal("MFA success result")
			}
			_, me := user.call("GET", "/auth/me", nil, 200)
			id := me["account"].(map[string]any)["id"].(string)
			// Even without optional personal-role permissions, own MFA remains available.
			if _, e = env.Pool.Exec(ctx, `DELETE FROM account_roles WHERE account_id=$1`, id); e != nil {
				t.Fatal(e)
			}
			user.call("GET", "/account/mfa", nil, 200)
			user.call("GET", "/account/profile", nil, 403)
			user.call("POST", "/account/mfa/recovery-codes/regenerate", map[string]string{"expected_factor_version": "2", "expected_recovery_generation": "1", "totp_code": "123456"}, 400)
			user.call("POST", "/account/mfa/recovery-codes/regenerate", map[string]string{"expected_factor_version": "2", "expected_recovery_generation": "1"}, 200)
			_, status := admin.call("GET", "/access/accounts/"+id+"/mfa", nil, 200)
			if _, ok := status["secret"]; ok {
				t.Fatal("admin factor exposure")
			}
			_, reset := admin.call("POST", "/access/accounts/"+id+"/mfa-reset", map[string]string{"expected_factor_version": "2", "reason": "身份已核验"}, 200)
			user.call("GET", "/auth/me", nil, 401)
			_, challenge = user.call("POST", "/auth/login", map[string]string{"username": "MfaUser", "password": password}, 202)
			if challenge["status"] != "mfa_recovery_required" {
				t.Fatal("reset bypass in optional mode")
			}
			grant, _ := url.Parse(reset["recovery_url"].(string))
			values, _ := url.ParseQuery(grant.Fragment)
			user.call("POST", "/auth/mfa/recovery/claim", map[string]string{"challenge_id": challenge["challenge_id"].(string), "recovery_token": values.Get("token")}, 202)
			user.call("POST", "/auth/logout", nil, 204)
			_, none := user.call("GET", "/auth/mfa/challenge", nil, 200)
			if none["status"] != "none" {
				t.Fatal("logout left challenge")
			}
			for _, path := range []string{"/auth/mfa/verify", "/auth/mfa/recovery/claim", "/account/mfa/enrollment/confirm"} {
				request, _ := http.NewRequest("POST", server.URL+"/api/v1"+path, strings.NewReader(`{}`))
				request.Header.Set("Origin", env.Config.TrustedOrigins[0])
				resp, e := user.client.Do(request)
				if e != nil {
					t.Fatal(e)
				}
				resp.Body.Close()
				if resp.StatusCode != 403 {
					t.Fatal("missing CSRF accepted", path)
				}
			}
			user.call("POST", "/auth/mfa/verify", map[string]string{"challenge_id": uuid.NewString(), "method": "totp", "code": strings.Repeat("0", 17_000)}, 400)
			origin := user.origin
			user.origin = "https://attacker.example"
			user.call("POST", "/auth/mfa/verify", map[string]string{"challenge_id": uuid.NewString(), "method": "totp", "code": "000000"}, 403)
			user.origin = origin
			_, failedChallenge := user.call("POST", "/auth/login", map[string]string{"username": "MfaUser", "password": password}, 202)
			for range 5 {
				user.call("POST", "/auth/mfa/recovery/claim", map[string]string{"challenge_id": failedChallenge["challenge_id"].(string), "recovery_token": "invalid"}, 400)
			}
			_, failedChallenge = user.call("POST", "/auth/login", map[string]string{"username": "MfaUser", "password": password}, 202)
			limited, failure := user.call("POST", "/auth/mfa/recovery/claim", map[string]string{"challenge_id": failedChallenge["challenge_id"].(string), "recovery_token": "invalid"}, 429)
			retry, e := strconv.Atoi(limited.Header.Get("Retry-After"))
			if e != nil || retry < 1 || retry > 31 || failure["code"] != "rate_limited" {
				t.Fatal("MFA cooldown response lost its durable retry interval")
			}
		})
	}
}
