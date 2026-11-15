// Package adapters implements only enumerated read-only checks. Raw upstream
// errors, response bodies, process argv and environments never become evidence.
package adapters

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type Collector interface {
	Collect(context.Context, settings.Probe, settings.Binding) model.Observation
}
type Runtime struct {
	Manifest settings.Manifest
	Bindings settings.Bindings
	BootID   string
	mu       sync.Mutex
	samples  map[string]sample
	// The systemd reader is an explicit seam for a private D-Bus fixture.
	Systemd PropertyReader
}
type sample struct {
	at                 time.Time
	identity           string
	cpu, total, rx, tx float64
}

func New(m settings.Manifest, b settings.Bindings, boot string) *Runtime {
	return &Runtime{Manifest: m, Bindings: b, BootID: boot, samples: map[string]sample{}}
}
func (r *Runtime) Collect(ctx context.Context, p settings.Probe, b settings.Binding) (out model.Observation) {
	started := time.Now()
	out = model.New(p, r.BootID, started)
	defer func() {
		if recover() != nil {
			out = model.Failure(model.New(p, r.BootID, started), "failed", "adapter_error")
			out.Facets = model.UnknownFacets()
		}
		if p.Adapter != "self_report" {
			ms := time.Since(started).Milliseconds()
			out.DurationMS = &ms
		}
	}()
	if b.ID == "" {
		out = model.Failure(out, "not_configured", "not_configured")
		out.Facets = model.UnknownFacets()
		return
	}
	switch p.Adapter {
	case "native_process":
		out = r.native(ctx, p, b, out, false)
	case "self_report":
		out = r.native(ctx, p, b, out, true)
	case "host_resources":
		out = r.resources(ctx, p, b, out)
	case "docker_container":
		out = r.docker(ctx, p, b, out)
	case "systemd_unit":
		out = r.systemd(ctx, p, b, out)
	default:
		out = r.protocol(ctx, p, b, out)
	}
	return
}
func classify(err error) (string, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return "timeout", "connection_timeout"
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, securefile.ErrUnsafe) {
		return "permission_denied", "permission_denied"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "failed", "connection_refused"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "failed", "dns_failed"
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return "failed", "tls_invalid"
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return "failed", "tls_invalid"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout", "connection_timeout"
	}
	return "failed", "adapter_error"
}
func failure(o model.Observation, err error) model.Observation {
	outcome, code := classify(err)
	return model.Failure(o, outcome, code)
}
func (r *Runtime) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, settings.Invalid
	}
	if !settings.TargetHost(host, r.Bindings.AllowedTargetHosts) {
		return nil, settings.Invalid
	}
	addrs, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, e
	}
	if len(addrs) == 0 {
		return nil, &net.DNSError{IsNotFound: true}
	}
	public := settings.OneOf(host, r.Bindings.AllowedTargetHosts...)
	for _, a := range addrs {
		if !settings.SafeIP(a.IP, public) {
			return nil, settings.Error("permission_denied")
		}
	}
	// One connection per attempt. DNS is pinned for this attempt, preventing
	// rebinding between validation and a driver's subsequent dial.
	return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
}
func tlsConfig(ca, host string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
	if ca != "" {
		raw, e := securefile.Read(ca, 1<<20, securefile.Policy{UID: os.Getuid(), Public: true})
		if e != nil {
			return nil, e
		}
		pool, e := x509.SystemCertPool()
		if e != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(raw) {
			return nil, settings.Error("tls_invalid")
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}
func (r *Runtime) httpClient(ca, host string) (*http.Client, error) {
	cfg, e := tlsConfig(ca, host)
	if e != nil {
		return nil, e
	}
	transport := &http.Transport{Proxy: nil, DialContext: r.dial, TLSClientConfig: cfg, DisableKeepAlives: true, MaxConnsPerHost: 1, ResponseHeaderTimeout: 2 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func credential(path, kind string) (settings.Credential, error) {
	var c settings.Credential
	if path == "" {
		return c, settings.Error("not_configured")
	}
	raw, e := securefile.Read(path, 4096, securefile.Private())
	if e != nil {
		return c, e
	}
	if settings.Strict(raw, &c) != nil || c.Validate() != nil || c.Kind != kind {
		return c, settings.Error("not_configured")
	}
	return c, nil
}
func readLimit(body io.Reader, n int64) ([]byte, error) {
	raw, e := io.ReadAll(io.LimitReader(body, n+1))
	if int64(len(raw)) > n {
		return nil, settings.Error("response_limit_exceeded")
	}
	return raw, e
}
func (r *Runtime) metricDelta(key string, next sample) (sample, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.samples[key]
	r.samples[key] = next
	if !ok || old.identity != next.identity || !next.at.After(old.at) {
		return sample{}, false
	}
	return old, true
}
func absentMetric(key, dimension, availability, reason string) model.Metric {
	return model.Metric{Key: key, DimensionID: dimension, Unit: model.MetricUnits[key], Availability: availability, ReasonCode: reason}
}
func metric(key, dimension string, value any) model.Metric {
	return model.Metric{Key: key, DimensionID: dimension, Unit: model.MetricUnits[key], Value: value, Availability: "available"}
}
func isNotConfigured(err error) bool { return strings.Contains(settings.Code(err), "not_configured") }
