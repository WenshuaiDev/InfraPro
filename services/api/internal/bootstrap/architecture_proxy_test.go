package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	public "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
)

// Exercise a real TLS hop, including the first response bytes before EOF.
// net/http deliberately normalizes transport framing. All end-to-end header
// values, Host, the escaped target, origin and input bytes must survive.
func TestArchitectureStreamingProxy(t *testing.T) {
	root := t.TempDir()
	roles := append(slices.Clone(config.Owners), config.Gateway)
	issueCertificates(t, root, roles)
	server := httptest.NewUnstartedServer(nil)
	a := config.Architecture{Endpoints: map[string]string{}, CertDirectory: root, Role: config.Transfer}
	for _, role := range config.Owners {
		a.Endpoints[role] = "https://" + server.Listener.Addr().String()
	}
	network, e := privateapi.Open(&a)
	if e != nil {
		t.Fatal(e)
	}
	defer network.Close()
	server.TLS = network.TLS
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var calls atomic.Int32
	const raw = "/api/v1/account/%61vatar?version=%2b&tag=a&tag=b"
	data := bytes.Repeat([]byte("0123456789abcdef"), 8192)
	sum := sha256.Sum256(data)
	server.Config.Handler = public.FromGateway(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.RawQuery == "ambiguous" {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		if r.URL.RawQuery == "cancel" {
			close(started)
			<-r.Context().Done()
			close(cancelled)
			return
		}
		payload, e := io.ReadAll(r.Body)
		host, _, peerErr := net.SplitHostPort(r.RemoteAddr)
		if e != nil || peerErr != nil || host != "127.0.0.1" || r.RequestURI != raw || r.Host != "console.test" || r.Header.Get("Origin") != "http://console.test" || r.Header.Get("X-Infrapro-Principal") != "" || r.Header.Get(privateapi.RequestID) == "forged" || privateapi.Trace(r.Context()) == "" || sha256.Sum256(payload) != sum {
			t.Error("gateway changed payload, original peer, target, trace or Host")
		}
		if !slices.Equal(r.Header.Values("X-Test-Input"), []string{"one", "two"}) {
			t.Error("duplicate input headers collapsed")
		}
		w.Header().Add("Set-Cookie", "first=value; Path=/; HttpOnly")
		w.Header().Add("Set-Cookie", "second=value; Path=/; SameSite=Lax")
		w.Header().Set("ETag", hex.EncodeToString(sum[:]))
		w.Header().Set("Content-Range", "bytes 0-10/42")
		w.WriteHeader(206)
		_, _ = fmt.Fprint(w, "first")
		http.NewResponseController(w).Flush()
		select {
		case <-r.Context().Done():
			return
		case <-started:
		}
		_, _ = fmt.Fprint(w, "second")
	}))
	server.StartTLS()
	defer server.Close()
	a.Role = config.Gateway
	out, e := privateapi.Open(&a)
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	cfg := config.Config{Architecture: &a, CSRFKey: []byte("0123456789abcdef0123456789abcdef"), TrustedOrigins: []string{"http://console.test"}}
	gateway := httptest.NewServer(public.Gateway(cfg, out, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer gateway.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	r, e := http.NewRequest("PUT", gateway.URL+raw, bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	r.Host = "console.test"
	r.Header.Set("Origin", "http://console.test")
	r.Header.Set(privateapi.RequestID, "forged")
	r.Header.Set("X-Infrapro-Principal", "forged")
	r.Header.Add("X-Test-Input", "one")
	r.Header.Add("X-Test-Input", "two")
	response, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	first := make([]byte, 5)
	if _, e = io.ReadFull(response.Body, first); e != nil || string(first) != "first" || response.StatusCode != 206 || response.Header.Get("Content-Range") != "bytes 0-10/42" || len(response.Cookies()) != 2 || response.Header.Get("ETag") != hex.EncodeToString(sum[:]) {
		t.Fatal("stream headers or first bytes lost")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stop, e := http.NewRequestWithContext(ctx, "GET", gateway.URL+"/api/v1/account/avatar?cancel", nil)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() {
		if v, _ := client.Do(stop); v != nil {
			v.Body.Close()
		}
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("second request did not reach upstream")
	}
	tail, e := io.ReadAll(response.Body)
	if e != nil || string(tail) != "second" {
		t.Fatal("response truncated")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not reach owner")
	}
	<-done
	if calls.Load() != 2 {
		t.Fatalf("unexpected replay (%d calls)", calls.Load())
	}
	// The owner accepted this request before its connection closed. Even a
	// caller-supplied idempotency header must not trigger a second audit/read.
	lost, err := http.NewRequestWithContext(t.Context(), "GET", gateway.URL+"/api/v1/account/avatar?ambiguous", nil)
	if err != nil {
		t.Fatal(err)
	}
	lost.Header.Set("Idempotency-Key", "ambiguous-result")
	unavailable, err := client.Do(lost)
	if err != nil {
		t.Fatal(err)
	}
	unavailable.Body.Close()
	if unavailable.StatusCode != 503 || calls.Load() != 3 {
		t.Fatalf("ambiguous result %d, %d executions", unavailable.StatusCode, calls.Load())
	}
	u, _ := url.Parse(a.Endpoints[config.Transfer])
	untrusted := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: network.TLS.RootCAs, ServerName: "transfer-api.infrapro.internal"}}, Timeout: 2 * time.Second}
	resp, e := untrusted.Post(u.String()+privateapi.SessionPath, "application/json", strings.NewReader("{}"))
	if resp != nil {
		resp.Body.Close()
	}
	if e == nil {
		t.Fatal("listener accepted a client without a private certificate")
	}
}
