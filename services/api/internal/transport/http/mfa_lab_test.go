package http_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/pquerna/otp/totp"
)

// Opt-in, isolated manual acceptance server. Both instances use independently
// assembled identity services over the same random schema, Redis prefix/keyring.
func TestMFAAcceptanceServer(t *testing.T) {
	if os.Getenv("INFRAPRO_MFA_LAB") != "1" {
		t.Skip("run pnpm mfa:lab")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	directory := os.Getenv("INFRAPRO_MFA_LAB_PRIVATE")
	if !filepath.IsAbs(directory) {
		t.Fatal("private absolute lab directory required")
	}
	env.Config.PublicConsoleURL = os.Getenv("INFRAPRO_E2E_CONSOLE_URL") + "/"
	env.Config.TrustedOrigins = []string{strings.TrimRight(env.Config.PublicConsoleURL, "/")}
	closeDisplay := browserDisplay(t, ctx, env, nil)
	defer closeDisplay()
	env.Service, e = identity.NewService(env.Pool, env.Redis, env.Config, logger)
	if e != nil {
		t.Fatal(e)
	}
	s := env.Service
	go runtime.ComposeNotifications(s.Access()).Run(ctx, "mfa-acceptance", func(error) { t.Log("notification worker temporarily unavailable") })
	credentials := map[string]map[string]string{}
	for _, label := range []string{"P1", "P2", "P3", "TA", "U1", "U2", "U3", "U4", "U5", "U6"} {
		credentials[label] = map[string]string{"username": "mfa_" + strings.ToLower(label), "password": rand.Text(), "label": label}
	}
	first := credentials["P1"]
	if _, e = s.Bootstrap(ctx, first["username"], "MFA P1 · 未绑定平台管理员", first["password"], identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	_, token, _, e := s.Login(ctx, first["username"], first["password"], identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	admin, e := s.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	first["id"] = admin.Account.ID
	for _, label := range []string{"P2", "P3", "TA", "U1", "U2", "U3", "U4", "U5", "U6"} {
		c := credentials[label]
		link, e := s.CreateInvitation(ctx, admin, "", "MFA 独立合成验收", identity.Meta{})
		if e != nil {
			t.Fatal(e)
		}
		if e = s.AcceptInvitation(ctx, strings.Split(link.URL, "#token=")[1], c["username"], "MFA "+label, c["password"], identity.Meta{}); e != nil {
			t.Fatal(e)
		}
		account, e := s.Find(ctx, c["username"])
		if e != nil {
			t.Fatal(e)
		}
		c["id"] = account.ID
		if label == "P2" || label == "U5" {
			_, token, _, e := s.Login(ctx, c["username"], c["password"], identity.Meta{})
			if e != nil {
				t.Fatal(e)
			}
			p, e := s.Authenticate(ctx, token)
			if e != nil {
				t.Fatal(e)
			}
			setup, e := s.StartSelfEnrollment(ctx, p, "enable", "1", identity.Meta{})
			if e != nil {
				t.Fatal(e)
			}
			code, _ := totp.GenerateCode(setup.Secret, time.Now().Add(-30*time.Second))
			if _, e = s.ConfirmEnrollment(ctx, &p, "", setup.ID, code, identity.Meta{}); e != nil {
				t.Fatal(e)
			}
			// Deliberately discard test factors/codes. Manual acceptance uses P1 to issue
			// a one-time recovery link, then binds the user's real authenticator in UI.
		}
	}
	if e = s.SetStatus(ctx, credentials["U5"]["username"], false, false, identity.Meta{Reason: "MFA 停用账号验收"}); e != nil {
		t.Fatal(e)
	}
	write := func(name string, value any) {
		t.Helper()
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		defer clear(raw)
		if e = os.WriteFile(filepath.Join(directory, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("credentials.json", credentials)
	write("operator.json", map[string]any{"database_url": env.Config.DatabaseURL, "redis_url": env.Config.RedisURL, "redis_key_prefix": env.Config.RedisKeyPrefix, "csrf_auth_key": hex.EncodeToString(env.Config.CSRFKey), "public_console_url": env.Config.PublicConsoleURL, "trusted_origins": strings.Join(env.Config.TrustedOrigins, ","), "support_contact": "MFA 隔离验收维护员", "mfa_keyring_file": env.Config.MFAKeyringFile})
	other, e := identity.NewService(env.Pool, env.Redis, env.Config, logger)
	if e != nil {
		t.Fatal(e)
	}
	failures := make(chan error, 2)
	servers := []*http.Server{}
	for i, service := range []*identity.Service{s, other} {
		address := os.Getenv([]string{"INFRAPRO_MFA_API_A", "INFRAPRO_MFA_API_B"}[i])
		listener, e := net.Listen("tcp", address)
		if e != nil {
			t.Fatal("MFA acceptance port unavailable")
		}
		server := &http.Server{Handler: httptransport.NewRouter(logger, service, env.Config), ReadHeaderTimeout: 5 * time.Second}
		servers = append(servers, server)
		go func() { failures <- server.Serve(listener) }()
		go service.RunJobs(ctx)
	}
	select {
	case e = <-failures:
		t.Fatal(e)
	case <-ctx.Done():
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, server := range servers {
		_ = server.Shutdown(closeCtx)
	}
}
