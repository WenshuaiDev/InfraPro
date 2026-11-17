package http_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

func TestMFACLIInteractiveAndNoFalseSession(t *testing.T) {
	binary := os.Getenv("INFRAPRO_MFA_CLI_BINARY")
	if binary == "" {
		t.Skip("run pnpm test:mfa")
	}
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	s := env.Service
	_, e = s.Bootstrap(ctx, "MfaOperator", "合成管理员", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, token, _, e := s.Login(ctx, "MfaOperator", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	link, e := s.CreateInvitation(ctx, p, "", "合成终端账号", identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AcceptInvitation(ctx, strings.Split(link.URL, "#token=")[1], "MfaCliUser", "", password, identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	setup, e := s.StartSelfEnrollment(ctx, p, "enable", "1", identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now().Add(-30*time.Second))
	bound, e := s.ConfirmEnrollment(ctx, &p, "", setup.ID, code, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Authenticate(ctx, bound.SessionToken)
	if e != nil {
		t.Fatal(e)
	}
	// The CLI is a separate OS process and exercises the public router/cookies.
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	// Use the actual loopback origin for CSRF, not the fixture's default UI URL.
	env.Config.TrustedOrigins = []string{origin}
	env.Config.PublicConsoleURL = origin + "/"
	server.Config.Handler = httptransport.NewRouter(logger, s, env.Config)
	server.Start()
	defer server.Close()
	_, source, _, _ := runtime.Caller(0)
	script := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../tests/mfa/cli.py"))
	run := func(cases []map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(cases)
		defer clear(raw)
		cmd := exec.CommandContext(ctx, "python3", script)
		cmd.Stdin = bytes.NewReader(raw)
		if _, e = cmd.CombinedOutput(); e != nil {
			t.Fatal("interactive CLI scenario failed; credentials and transcript withheld")
		}
	}
	input := func(prompt, value string) map[string]string {
		return map[string]string{"prompt": prompt, "value": value}
	}
	scenario := func(username string, inputs []map[string]string, success bool) map[string]any {
		path := filepath.Join(t.TempDir(), "session.json")
		return map[string]any{"args": []string{binary, "monitoring", "derived", "login", "--url", server.URL, "--username", username, "--session-file", path}, "inputs": inputs, "success": success, "session": path, "secrets": []string{password, setup.Secret}}
	}
	current, _ := totp.GenerateCode(setup.Secret, time.Now())
	run([]map[string]any{
		scenario("MfaCliUser", []map[string]string{input("登录密码：", password)}, true),
		scenario("MfaOperator", []map[string]string{input("登录密码：", password), input("认证方式", "1"), input("验证码（不回显）", current)}, true),
		scenario("MfaOperator", []map[string]string{input("登录密码：", password), input("认证方式", "2"), input("验证码（不回显）", bound.Result.RecoveryCodes[0])}, true),
	})
	if _, e = s.UpdatePolicy(ctx, p, true, "1", uuid.NewString(), identity.Meta{Reason: "CLI 强制绑定验收"}); e != nil {
		t.Fatal(e)
	}
	run([]map[string]any{scenario("MfaCliUser", []map[string]string{input("登录密码：", password)}, false)})
	cfg := map[string]any{"database_url": env.Config.DatabaseURL, "redis_url": env.Config.RedisURL, "redis_key_prefix": env.Config.RedisKeyPrefix, "csrf_auth_key": hex.EncodeToString(env.Config.CSRFKey), "public_console_url": server.URL + "/", "trusted_origins": server.URL, "support_contact": "隔离维护员", "mfa_keyring_file": env.Config.MFAKeyringFile}
	raw, _ := json.Marshal(cfg)
	file := filepath.Join(t.TempDir(), "operator.json")
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	clear(raw)
	run([]map[string]any{{"args": []string{binary, "account", "mfa-reset", "--username", "MfaOperator", "--reason", "唯一管理员合成恢复", "--operator", "测试维护员"}, "env": map[string]string{"INFRAPRO_CONFIG": file}, "success": true, "maintenance": true}})
	d, e := s.BeginLogin(ctx, "MfaOperator", password, "", identity.Meta{})
	if e != nil || d.Result.Status != "mfa_recovery_required" {
		t.Fatal("maintenance CLI bypassed recovery", e)
	}
}
