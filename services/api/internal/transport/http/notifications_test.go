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
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func TestNotificationHTTPPersonOwnershipCSRFAndIdle(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:notifications")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	if _, e = env.Service.Bootstrap(ctx, "InboxOwner", "收件人", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path string, body any, withCSRF bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Origin", env.Config.TrustedOrigins[0])
		req.Header.Set("Content-Type", "application/json")
		if withCSRF {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
			t.Fatal("private API cacheable")
		}
		v := map[string]any{}
		if res.StatusCode != 204 {
			if e = json.NewDecoder(res.Body).Decode(&v); e != nil {
				t.Fatal(e)
			}
		}
		return res.StatusCode, v
	}
	if code, _ := call("GET", "/me/notifications/summary", nil, false); code != 401 {
		t.Fatal(code)
	}
	_, v := call("GET", "/auth/csrf", nil, false)
	csrf = v["token"].(string)
	if code, v := call("POST", "/auth/login", map[string]string{"username": "InboxOwner", "password": password}, true); code != 200 {
		t.Fatal(code, v)
	}
	_, v = call("POST", "/invitations", map[string]string{"note": "通知真实开户结果"}, true)
	token := strings.Split(v["url"].(string), "#token=")[1]
	// A separate unauthenticated principal consumes the real invitation.
	if e = env.Service.AcceptInvitation(ctx, token, "InboxOther", "其他账号", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	s := runtime.ComposeNotifications(env.Service.Access())
	if _, e = s.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var before, after time.Time
	if e = env.Pool.QueryRow(ctx, `SELECT last_activity_at FROM account_sessions ORDER BY created_at DESC LIMIT 1`).Scan(&before); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/me/notifications", "/me/notifications/summary", "/me/notification-preferences"} {
		if code, v := call("GET", path, nil, false); code != 200 {
			t.Fatal(code, v)
		}
	}
	if e = env.Pool.QueryRow(ctx, `SELECT last_activity_at FROM account_sessions ORDER BY created_at DESC LIMIT 1`).Scan(&after); e != nil || !before.Equal(after) {
		t.Fatal("background polling extended idle", e)
	}
	_, v = call("GET", "/me/notifications", nil, false)
	items := v["items"].([]any)
	if len(items) != 1 {
		t.Fatal(v)
	}
	item := items[0].(map[string]any)
	id := item["id"].(string)
	if code, _ := call("PATCH", "/me/notifications/"+id, map[string]any{"expected_revision": item["revision"], "read": true}, false); code != 403 {
		t.Fatal("CSRF bypass", code)
	}
	if code, _ := call("GET", "/me/notifications?account_id=other", nil, false); code != 400 {
		t.Fatal("account selector", code)
	}
	if code, _ := call("PATCH", "/me/notifications/"+id, map[string]any{"expected_revision": item["revision"], "read": true, "account_id": "other"}, true); code != 400 {
		t.Fatal("body identity override", code)
	}
	if code, _ := call("PATCH", "/me/notifications/"+id, map[string]any{"expected_revision": item["revision"], "read": true}, true); code != 200 {
		t.Fatal(code)
	}
	if code, _ := call("GET", "/me/notifications/"+id+"/target?return_to=https://example.invalid", nil, false); code != 400 {
		t.Fatal("redirect injection", code)
	}
	call("POST", "/auth/logout", nil, true)
	if code, v := call("POST", "/auth/login", map[string]string{"username": "InboxOther", "password": password}, true); code != 200 {
		t.Fatal(code, v)
	}
	for _, path := range []string{"/me/notifications/" + id, "/me/notifications/" + id + "/target"} {
		if code, _ := call("GET", path, nil, false); code != 404 {
			t.Fatal("cross-person data", code)
		}
	}
	if code, _ := call("PATCH", "/me/notifications/"+id, map[string]any{"expected_revision": "1", "read": false}, true); code != 404 {
		t.Fatal("cross-person mutation", code)
	}
	if code, v := call("GET", "/me/notifications/summary", nil, false); code != 200 || v["unread"] != float64(0) {
		t.Fatal("cross-person count", code, v)
	}
}
