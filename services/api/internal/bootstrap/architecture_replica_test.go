package bootstrap_test

import (
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
)

func TestArchitectureReplicaMTLS(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL and Redis")
	}
	ctx := t.Context()
	env, err := testsupport.New(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	const password = "IsolatedReplicaTestPassword159"
	account, err := env.Service.Bootstrap(ctx, "replicatest", "副本自验", password, identity.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	_, cookie, _, err := env.Service.Login(ctx, "replicatest", password, identity.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	dir := t.TempDir()
	slots := map[string][]string{config.Control: {"control-api-1", "control-api-2"}, config.Identity: {"identity-api-1", "identity-api-2"}}
	names := map[string][]string{
		config.Control: {"control-api-1", config.Control}, config.Identity: {config.Identity, "identity-api-1"},
		"other-role": {config.Control, config.Transfer}, "foreign-slot": {config.Control, "control-api-3"},
		"missing-slot": {config.Control}, "wrong-server": {config.Identity, "identity-api-3"},
	}
	issueCertificates(t, dir, []string{config.Identity, config.Control, "other-role", "foreign-slot", "missing-slot", "wrong-server"}, names)
	a := config.Architecture{Role: config.Identity, ReplicaID: "identity-api-1", CertDirectory: dir, ReplicaSlots: slots, Endpoints: map[string]string{config.Identity: "https://" + server.Listener.Addr().String()}}
	owner, err := privateapi.Open(&a)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	server.Config.Handler = owner.RegisteredPeer(privateapi.VerifyHandler(env.Service))
	server.TLS = owner.TLS
	server.StartTLS()
	b := a
	b.Role, b.ReplicaID = config.Control, "control-api-1"
	caller, err := privateapi.Open(&b)
	if err != nil {
		t.Fatal(err)
	}
	defer caller.Close()
	traced := privateapi.WithTrace(ctx, "replica-capacity-test")
	route := privateapi.SessionRoute(traced, "GET", "/api/v1/platform/engineering/unit-categories")
	p, err := caller.Verify(route, cookie)
	if err != nil || p.Account.ID != account.ID {
		t.Fatal("registered client/server replicas did not verify a real session")
	}
	differentOwner := privateapi.SessionRoute(traced, "GET", "/api/v1/auth/me")
	if _, err = caller.Verify(differentOwner, cookie); err == nil {
		t.Fatal("replica gained authority over a different role's operation")
	}
	for _, name := range []string{"other-role", "foreign-slot", "missing-slot"} {
		cert, err := tls.LoadX509KeyPair(filepath.Join(dir, name+".pem"), filepath.Join(dir, name+".key"))
		if err != nil {
			t.Fatal(err)
		}
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: owner.TLS.RootCAs, ServerName: config.Identity + ".infrapro.internal", Certificates: []tls.Certificate{cert}}}
		guest := &http.Client{Transport: transport}
		resp, err := guest.Get(server.URL)
		transport.CloseIdleConnections()
		if err == nil {
			resp.Body.Close()
			t.Fatalf("server accepted invalid role or slot %s", name)
		}
	}
	impostor := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer impostor.Close()
	impostor.TLS = owner.TLS.Clone()
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "wrong-server.pem"), filepath.Join(dir, "wrong-server.key"))
	if err != nil {
		t.Fatal(err)
	}
	impostor.TLS.Certificates = []tls.Certificate{cert}
	impostor.StartTLS()
	unregistered := b
	unregistered.Endpoints = map[string]string{config.Identity: impostor.URL}
	toImpostor, err := privateapi.Open(&unregistered)
	if err != nil {
		t.Fatal(err)
	}
	defer toImpostor.Close()
	// Use the genuine network's client TLS verifier, independently of response decoding.
	c := toImpostor.TLS.Clone()
	c.ServerName = config.Identity + ".infrapro.internal"
	transport := &http.Transport{TLSClientConfig: c}
	defer transport.CloseIdleConnections()
	resp, err := (&http.Client{Transport: transport}).Get(impostor.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("client accepted an unregistered server slot")
	}
}
