package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/bootstrap"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
	public "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

func issueCertificates(t *testing.T, dir string, roles []string, slots ...map[string][]string) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated HTTP test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	encode := func(name, kind string, data []byte) {
		if e := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}), 0600); e != nil {
			t.Fatal(e)
		}
	}
	cert, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	encode("ca.pem", "CERTIFICATE", cert)
	for i, role := range roles {
		pub, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, DNSNames: []string{role + ".infrapro.internal"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		if len(slots) != 0 && len(slots[0][role]) != 0 {
			template.DNSNames = nil
			for _, id := range slots[0][role] {
				template.DNSNames = append(template.DNSNames, id+".infrapro.internal")
			}
		}
		der, e := x509.CreateCertificate(rand.Reader, template, ca, pub, key)
		if e != nil {
			t.Fatal(e)
		}
		encode(role+".pem", "CERTIFICATE", der)
		raw, e := x509.MarshalPKCS8PrivateKey(priv)
		if e != nil {
			t.Fatal(e)
		}
		encode(role+".key", "PRIVATE KEY", raw)
	}
}
func address(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().String()
}

type httpLab struct {
	old, gateway string
	cfg          config.Config
	arch         config.Architecture
	stop         map[string]context.CancelFunc
}

func startLab(t *testing.T, env *testsupport.Environment, degraded bool) httpLab {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	baseline := httptest.NewServer(public.NewRouter(logger, env.Service, env.Config))
	t.Cleanup(baseline.Close)
	roles := append(slices.Clone(config.Owners), config.Gateway, "display-runtime", "diagnostics-observer")
	a := config.Architecture{CertDirectory: t.TempDir(), Endpoints: map[string]string{}}
	issueCertificates(t, a.CertDirectory, roles)
	for _, owner := range config.Owners {
		a.Endpoints[owner] = "https://" + address(t)
	}
	a.Endpoints[config.Gateway] = "https://" + address(t)
	gatewayAddr := address(t)
	lab := httpLab{old: baseline.URL, gateway: "http://" + gatewayAddr, cfg: env.Config, arch: a, stop: map[string]context.CancelFunc{}}
	for _, role := range append(slices.Clone(config.Owners), config.Gateway) {
		c := env.Config
		local := a
		local.Role = role
		c.Architecture = &local
		c.ShutdownTimeout = 4 * time.Second
		if role != config.Identity {
			c.MFAKeyringFile = filepath.Join(t.TempDir(), "absent")
		}
		if role == config.Gateway {
			c.HTTPAddr = gatewayAddr
			c.DatabaseURL = ""
			c.RedisURL = ""
		}
		ctx, cancel := context.WithCancel(t.Context())
		lab.stop[role] = cancel
		done := make(chan error, 1)
		go func() { done <- bootstrap.Run(ctx, c, logger) }()
		t.Cleanup(func() {
			cancel()
			select {
			case e := <-done:
				if e != nil {
					t.Errorf("%s shutdown: %v", role, e)
				}
			case <-time.After(7 * time.Second):
				t.Errorf("%s shutdown timeout", role)
			}
		})
	}
	client := &http.Client{Timeout: time.Second}
	end := time.Now().Add(20 * time.Second)
	path, status := "/readyz", 200
	if degraded {
		path = "/api/v1/public/account-help"
	}
	for time.Now().Before(end) {
		r, e := client.Get(lab.gateway + path)
		if e == nil {
			r.Body.Close()
			if r.StatusCode == status {
				return lab
			}
		}
		time.Sleep(75 * time.Millisecond)
	}
	t.Fatal("split topology did not become ready")
	return lab
}

type browser struct {
	client             *http.Client
	base, origin, csrf string
}

func newBrowser(base, origin string) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{client: &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, base: base, origin: origin}
}

type response struct {
	Code   int
	Header http.Header
	Body   []byte
}

func (b *browser) do(t *testing.T, method, path, payload string) response {
	t.Helper()
	r, e := http.NewRequestWithContext(t.Context(), method, b.base+path, strings.NewReader(payload))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", b.origin)
	if b.csrf != "" {
		r.Header.Set("X-CSRF-Token", b.csrf)
	}
	r.Header.Set(privateapi.RequestID, "browser-supplied")
	r.Header.Set(privateapi.PeerAddress, "192.0.2.50:9999")
	reply, e := b.client.Do(r)
	if e != nil {
		t.Fatalf("%s %s: %v", method, path, e)
	}
	defer reply.Body.Close()
	body, e := io.ReadAll(reply.Body)
	if e != nil {
		t.Fatal(e)
	}
	return response{reply.StatusCode, reply.Header, body}
}
func (b *browser) prepare(t *testing.T) {
	t.Helper()
	reply := b.do(t, "GET", "/api/v1/auth/csrf", "")
	var body struct{ Token string }
	if reply.Code != 200 || json.Unmarshal(reply.Body, &body) != nil || body.Token == "" {
		t.Fatalf("csrf bootstrap: %d", reply.Code)
	}
	b.csrf = body.Token
}
func comparable(t *testing.T, r response, path string) response {
	t.Helper()
	r.Header = r.Header.Clone()
	id := r.Header.Get("X-Request-Id")
	if id == "" || id == "browser-supplied" {
		t.Fatal("invalid request ID")
	}
	r.Header.Del("X-Request-Id")
	r.Header.Del("Date")
	r.Body = bytes.ReplaceAll(r.Body, []byte(id), []byte("PAIRED_REQUEST_ID"))
	if path == "/api/v1/auth/csrf" && r.Code == 200 {
		var m map[string]string
		if json.Unmarshal(r.Body, &m) != nil || len(m["token"]) < 32 {
			t.Fatal("invalid CSRF token")
		}
		r.Body = bytes.ReplaceAll(r.Body, []byte(m["token"]), []byte("PAIRED_CSRF_TOKEN"))
	}
	return r
}

func TestArchitectureHTTP(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL and Redis")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	env, e := testsupport.New(t.Context(), logger)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(env.Close)
	if _, e = env.Service.Bootstrap(t.Context(), "archadmin", "验收管理员", "ArchitectureVerification492", identity.Meta{}); e != nil {
		t.Fatal(e)
	}
	lab := startLab(t, env, false)
	left, right := newBrowser(lab.old, env.Config.TrustedOrigins[0]), newBrowser(lab.gateway, env.Config.TrustedOrigins[0])
	left.prepare(t)
	right.prepare(t)
	compare := func(method, path, body string) response {
		t.Helper()
		a, b := comparable(t, left.do(t, method, path, body), path), comparable(t, right.do(t, method, path, body), path)
		if a.Code != b.Code || !bytes.Equal(a.Body, b.Body) || !reflect.DeepEqual(a.Header, b.Header) {
			t.Fatalf("%s %s differed\nA %d %q %v\nB %d %q %v", method, path, a.Code, a.Body, safeHeaders(a.Header), b.Code, b.Body, safeHeaders(b.Header))
		}
		return b
	}
	param := regexp.MustCompile(`:[a-zA-Z_][a-zA-Z0-9_]*`)
	var records []map[string]any
	for _, op := range routes.Operations {
		path := param.ReplaceAllString(op.Path, "00000000-0000-4000-8000-000000000001")
		payload := ""
		if op.Method != "GET" && op.Method != "HEAD" {
			payload = "{}"
		}
		r := compare(op.Method, path, payload)
		records = append(records, map[string]any{"method": op.Method, "template": op.Path, "owner": op.Owner, "operation_id": op.OperationID, "principal": "anonymous", "status": r.Code})
	}
	for _, extra := range [][2]string{{"GET", "/unknown"}, {"OPTIONS", "/api/v1/account/avatar"}, {"GET", "/api/v1/me/workspaces/"}, {"POST", "/readyz/"}, {"GET", "/internal/v1/sessions/verify"}} {
		compare(extra[0], extra[1], "")
	}
	left.origin = "http://invalid.test"
	right.origin = left.origin
	compare("POST", "/unknown", "{}")
	left.origin = env.Config.TrustedOrigins[0]
	right.origin = left.origin
	login := left.do(t, "POST", "/api/v1/auth/login", `{"username":"archadmin","password":"ArchitectureVerification492"}`)
	if login.Code != 200 {
		t.Fatalf("login %d %s", login.Code, login.Body)
	}
	src, _ := url.Parse(lab.old)
	dst, _ := url.Parse(lab.gateway)
	var token string
	for _, c := range left.client.Jar.Cookies(src) {
		if c.Name == "infrapro_session" {
			right.client.Jar.SetCookies(dst, []*http.Cookie{c})
			token = c.Value
		}
	}
	for _, sample := range [][2]string{{"GET", "/api/v1/me/workspaces"}, {"GET", "/api/v1/me/notifications"}, {"GET", "/api/v1/account/avatar"}, {"GET", "/does-not-exist"}, {"OPTIONS", "/api/v1/account/avatar"}} {
		compare(sample[0], sample[1], "")
	}
	local := lab.arch
	local.Role = config.Control
	verifier, err := privateapi.Open(&local)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	ctx := privateapi.WithTrace(t.Context(), "bounded-session-check")
	proof, err := verifier.Verify(privateapi.SessionRoute(ctx, "GET", "/api/v1/me/notifications"), token)
	if err != nil {
		t.Fatalf("control session proof: %v", err)
	}
	actor := identity.TransactionActor(proof, env.Config.SessionIdle)
	checkProof := func() error {
		tx, err := env.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		return actor.Check(ctx, tx, false)
	}
	if err := checkProof(); err != nil {
		t.Fatalf("current session rejected by owner: %v", err)
	}
	if _, err := verifier.Verify(privateapi.SessionRoute(ctx, "GET", "/api/v1/account/avatar"), token); !errors.Is(err, identity.ErrUnavailable) {
		t.Fatal("control role requested a transfer operation proof")
	}
	if r := right.do(t, "POST", "/api/v1/auth/logout", "{}"); r.Code != 204 {
		t.Fatalf("logout %d", r.Code)
	}
	if err := checkProof(); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("revoked cached proof reached transaction: %v", err)
	}
	for _, owner := range config.Owners {
		for _, op := range routes.Operations {
			if op.Owner == owner && op.Method == "GET" && op.Class != "public" && op.Class != "credential" {
				r := compare("GET", param.ReplaceAllString(op.Path, "00000000-0000-4000-8000-000000000001"), "")
				if r.Code != 401 {
					t.Fatalf("revoked session at %s got %d", owner, r.Code)
				}
				break
			}
		}
	}
	if file := os.Getenv("INFRAPRO_ARCHITECTURE_ROUTE_EVIDENCE"); file != "" {
		data, _ := json.MarshalIndent(records, "", "  ")
		if e = os.WriteFile(file, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Run("private caller isolation", func(t *testing.T) {
		local := lab.arch
		local.Role = "display-runtime"
		client, e := privateapi.Open(&local)
		if e != nil {
			t.Fatal(e)
		}
		defer client.Close()
		if _, e = client.Verify(t.Context(), "invalid"); e == nil || e != identity.ErrUnavailable {
			t.Fatal("unauthorized machine could reach session verifier")
		}
	})
	lab.stop[config.Query]()
	time.Sleep(250 * time.Millisecond)
	if r := right.do(t, "GET", "/readyz", ""); r.Code != 503 {
		t.Fatalf("outage readiness %d", r.Code)
	}
	if r := right.do(t, "GET", "/api/v1/public/account-help", ""); r.Code != 200 {
		t.Fatalf("healthy identity during query outage %d", r.Code)
	}
}

func safeHeaders(h http.Header) http.Header {
	v := h.Clone()
	if values := v.Values("Set-Cookie"); len(values) > 0 {
		v.Set("Set-Cookie", fmt.Sprintf("%d cookie(s)", len(values)))
	}
	return v
}
