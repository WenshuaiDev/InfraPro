// Package privateapi authenticates a service at both ends of each internal hop.
// The public Host header and personnel cookies are never machine identities.
package privateapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
)

const suffix = ".infrapro.internal"
const RequestID = "X-Infrapro-Request-Id"
const PeerAddress = "X-Infrapro-Peer"

type traceKey struct{}

func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}
func Trace(ctx context.Context) string { v, _ := ctx.Value(traceKey{}).(string); return v }

type Network struct {
	Arch    *config.Architecture
	TLS     *tls.Config
	clients map[string]*http.Client
	proxies map[string]*proxyPool
}

func Open(a *config.Architecture) (*Network, error) {
	pem, err := os.ReadFile(filepath.Join(a.CertDirectory, "ca.pem"))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid private CA")
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(a.CertDirectory, a.Role+".pem"), filepath.Join(a.CertDirectory, a.Role+".key"))
	if err != nil {
		return nil, err
	}
	if r, id := certificateIdentity(cert.Leaf.DNSNames, a); r != a.Role || id != "" && id != a.ReplicaID {
		return nil, errors.New("wrong local role or replica certificate")
	}
	base := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, RootCAs: pool, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
	base.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.VerifiedChains) != 0 && len(state.VerifiedChains[0]) != 0 {
			if r, _ := certificateIdentity(state.VerifiedChains[0][0].DNSNames, a); r != "" {
				return nil
			}
		}
		return errors.New("unregistered private role or replica certificate")
	}
	n := &Network{Arch: a, TLS: base, clients: map[string]*http.Client{}, proxies: map[string]*proxyPool{}}
	for role := range a.Endpoints {
		tc := base.Clone()
		tc.ServerName = role + suffix
		transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSClientConfig: tc, MaxConnsPerHost: a.Inflight(role), MaxIdleConnsPerHost: a.Inflight(role), IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 5 * time.Second, DisableCompression: true}
		if a.Role == config.Gateway {
			n.proxies[role] = newProxyPool(transport, a.Address(role), a.Inflight(role))
		}
		n.clients[role] = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return n, nil
}
func (n *Network) Close() {
	for _, p := range n.proxies {
		p.close()
	}
	for _, c := range n.clients {
		c.CloseIdleConnections()
	}
}
func (n *Network) Transport(role string) http.RoundTripper { return n.proxies[role] }

type peerKey struct{}

// LimitHandlers bounds a process across all of its listeners. A stream holds
// its slot until the handler exits, even if the headers were sent earlier.
func LimitHandlers(limit int) func(http.Handler) http.Handler {
	slots := make(chan struct{}, limit)
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				h.ServeHTTP(w, r)
			case <-r.Context().Done():
			}
		})
	}
}

func certificateIdentity(names []string, a *config.Architecture) (string, string) {
	if len(names) < 1 || len(names) > 2 {
		return "", ""
	}
	known := func(name string) bool {
		return slices.Contains(config.Owners, name) || name == config.Gateway || name == "compat-api" ||
			name == "display-runtime" || name == "diagnostics-observer" || name == "monitoring-incremental-worker" ||
			a != nil && (name == a.Role || a.Endpoints[name] != "" || len(a.ReplicaSlots[name]) != 0)
	}
	var role, slot string
	for _, dns := range names {
		name, ok := strings.CutSuffix(dns, suffix)
		if !ok || name == "" {
			return "", ""
		}
		if known(name) {
			if role != "" {
				return "", ""
			}
			role = name
		} else {
			slot = name
		}
	}
	if role == "" {
		return "", ""
	}
	var slots []string
	if a != nil {
		slots = a.ReplicaSlots[role]
	}
	if slot == "" && len(slots) == 0 || slot != "" && slices.Contains(slots, slot) {
		return role, slot
	}
	return "", ""
}

// RegisteredPeer correlates the role and optional slot in the verified leaf.
// Authorization uses this resolved role regardless of the SAN order.
func (n *Network) RegisteredPeer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.VerifiedChains) != 0 && len(r.TLS.VerifiedChains[0]) != 0 {
			role, _ := certificateIdentity(r.TLS.VerifiedChains[0][0].DNSNames, n.Arch)
			if role != "" {
				r = r.WithContext(context.WithValue(r.Context(), peerKey{}, role))
				h.ServeHTTP(w, r)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func Allow(roles []string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.VerifiedChains) > 0 && len(r.TLS.VerifiedChains[0]) > 0 {
			leaf := r.TLS.VerifiedChains[0][0]
			now := time.Now()
			if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
				http.NotFound(w, r)
				return
			}
			role, ok := r.Context().Value(peerKey{}).(string)
			if !ok {
				role, _ = certificateIdentity(leaf.DNSNames, nil)
			}
			if role != "" && slices.Contains(roles, role) {
				h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), peerKey{}, role)))
				return
			}
		}
		http.NotFound(w, r)
	})
}
func Read(w http.ResponseWriter, r *http.Request, max int64, v any) bool {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid private request", 400)
		return false
	}
	return true
}
func Write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func (n *Network) Call(ctx context.Context, role, path string, in, out any, max int64) error {
	client := n.clients[role]
	if client == nil {
		return errors.New("private endpoint unavailable")
	}
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", n.Arch.Endpoints[role]+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(RequestID, Trace(ctx))
	resp, err := client.Do(r)
	if err != nil {
		return errors.New("private transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("private operation unavailable")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil || int64(len(b)) > max {
		return errors.New("invalid private response")
	}
	return json.Unmarshal(b, out)
}
