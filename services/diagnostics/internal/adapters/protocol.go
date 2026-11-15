package adapters

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

func (r *Runtime) protocol(ctx context.Context, p settings.Probe, b settings.Binding, o model.Observation) model.Observation {
	o.Evidence.Scope = "none"
	o.Facets.Readiness = "not_ready"
	var err error
	switch b.Adapter {
	case "http_live", "http_ready":
		err = r.httpCheck(ctx, b, &o)
	case "tcp", "keeper_live":
		if b.Adapter == "keeper_live" && !b.Enabled {
			return model.Failure(o, "not_configured", "not_configured")
		}
		var conn net.Conn
		conn, err = r.dial(ctx, "tcp", net.JoinHostPort(b.Host, strconv.Itoa(b.Port)))
		if err == nil {
			defer conn.Close()
			deadline, _ := ctx.Deadline()
			_ = conn.SetDeadline(deadline)
			if b.TLS {
				cfg, e := tlsConfig(b.TLSCAFile, b.Host)
				if b.ServerName != "" && cfg != nil {
					cfg.ServerName = b.ServerName
				}
				if e == nil {
					secured := tls.Client(conn, cfg)
					e = secured.HandshakeContext(ctx)
				}
				err = e
			}
			if err == nil && b.Adapter == "keeper_live" {
				_, err = io.WriteString(conn, "ruok")
				if err == nil {
					var raw [5]byte
					var n int
					n, err = conn.Read(raw[:])
					if err == nil && string(raw[:n]) != "imok" {
						err = settings.Error("response_mismatch")
					}
				}
			}
		}
		o.Facets.Readiness = "unknown"
	case "postgres":
		err = r.postgres(ctx, b)
	case "redis":
		err = r.redis(ctx, b)
	case "clickhouse":
		err = r.clickhouse(ctx, b)
	case "s3":
		err = r.s3(ctx, b)
	default:
		return model.Failure(o, "unsupported", "adapter_unsupported")
	}
	if err != nil {
		outcome, code := protocolError(err)
		o = model.Failure(o, outcome, code)
		if outcome == "permission_denied" || outcome == "not_configured" || outcome == "unsupported" {
			o.Facets.Readiness = "unknown"
		}
		return o
	}
	if b.Adapter == "http_live" || b.Adapter == "tcp" || b.Adapter == "keeper_live" {
		o.Facets.Readiness = "not_applicable"
		if b.Adapter == "tcp" {
			o.Facets.Readiness = "unknown"
		}
	} else {
		o.Facets.Readiness = "ready"
	}
	return o
}
func protocolError(err error) (string, string) {
	var code settings.Error
	if errors.As(err, &code) {
		switch string(code) {
		case "not_configured":
			return "not_configured", "not_configured"
		case "permission_denied":
			return "permission_denied", "permission_denied"
		case "adapter_unsupported":
			return "unsupported", "adapter_unsupported"
		case "authentication_failed", "response_mismatch", "tls_invalid", "response_limit_exceeded":
			return "failed", string(code)
		}
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if strings.HasPrefix(pg.Code, "28") {
			return "failed", "authentication_failed"
		}
		if pg.Code == "42501" {
			return "permission_denied", "permission_denied"
		}
	}
	if redis.HasErrorPrefix(err, "NOAUTH") || redis.HasErrorPrefix(err, "WRONGPASS") {
		return "failed", "authentication_failed"
	}
	if redis.HasErrorPrefix(err, "NOPERM") {
		return "permission_denied", "permission_denied"
	}
	var ch *clickhouse.Exception
	if errors.As(err, &ch) {
		if ch.Code == 516 || ch.Code == 192 {
			return "failed", "authentication_failed"
		}
		if ch.Code == 497 || ch.Code == 452 || ch.Code == 164 {
			return "permission_denied", "permission_denied"
		}
	}
	s3 := minio.ToErrorResponse(err)
	if s3.Code == "InvalidAccessKeyId" || s3.Code == "SignatureDoesNotMatch" {
		return "failed", "authentication_failed"
	}
	if s3.StatusCode == 403 || s3.Code == "AccessDenied" {
		return "permission_denied", "permission_denied"
	}
	return classify(err)
}
func (r *Runtime) httpCheck(ctx context.Context, b settings.Binding, o *model.Observation) error {
	u, e := settings.TargetURL(b.URL, r.Bindings.AllowedTargetHosts)
	if e != nil {
		return e
	}
	client, e := r.httpClient(b.TLSCAFile, u.Hostname())
	if e != nil {
		return e
	}
	defer client.CloseIdleConnections()
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return e
	}
	if b.CredentialFile != "" {
		c, e := credential(b.CredentialFile, "bearer")
		if e != nil {
			return e
		}
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	response, e := client.Do(request)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	status := response.StatusCode
	o.Evidence.HTTPStatus = &status
	if status == 401 {
		return settings.Error("authentication_failed")
	}
	if status == 403 {
		return settings.Error("permission_denied")
	}
	if status != b.ExpectedStatus {
		return settings.Error("response_mismatch")
	}
	raw, e := readLimit(response.Body, 8192)
	if e != nil {
		return e
	}
	if len(b.ExpectedJSON) > 0 {
		var actual map[string]json.RawMessage
		if json.Unmarshal(raw, &actual) != nil {
			return settings.Error("response_mismatch")
		}
		for k, want := range b.ExpectedJSON {
			var a, v any
			if json.Unmarshal(actual[k], &a) != nil || json.Unmarshal(want, &v) != nil || a != v {
				return settings.Error("response_mismatch")
			}
		}
	}
	return nil
}
func (r *Runtime) postgres(ctx context.Context, b settings.Binding) error {
	cred, e := credential(b.CredentialFile, "password")
	if e != nil {
		return e
	}
	// Explicit fields replace all parser defaults. Environment pgpass/service
	// settings and fallback hosts never become a source of diagnostic credentials.
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(b.Host, strconv.Itoa(b.Port)), Path: "/" + b.Database, User: url.User(b.Username), RawQuery: "sslmode=disable&connect_timeout=2"}
	cfg, e := pgx.ParseConfig(u.String())
	if e != nil {
		return settings.Invalid
	}
	cfg.Host = b.Host
	cfg.Port = uint16(b.Port)
	cfg.Database = b.Database
	cfg.User = b.Username
	cfg.Password = cred.Password
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{"statement_timeout": "2000", "default_transaction_read_only": "on"}
	cfg.ConnectTimeout = 2 * time.Second
	cfg.DialFunc = r.dial
	cfg.LookupFunc = func(context.Context, string) ([]string, error) { return []string{b.Host}, nil }
	cfg.TLSConfig = nil
	if b.TLSMode == "verify-full" {
		cfg.TLSConfig, e = tlsConfig(b.TLSCAFile, b.Host)
		if e != nil {
			return e
		}
	}
	conn, e := pgx.ConnectConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Close(closing)
	}()
	var one int
	e = conn.QueryRow(ctx, "SELECT 1").Scan(&one)
	if e == nil && one != 1 {
		return settings.Error("response_mismatch")
	}
	return e
}
func (r *Runtime) redis(ctx context.Context, b settings.Binding) error {
	password := ""
	if b.CredentialFile != "" {
		c, e := credential(b.CredentialFile, "password")
		if e != nil {
			return e
		}
		password = c.Password
	}
	database, _ := strconv.Atoi(b.Database)
	safeDial := func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := r.dial(ctx, network, address)
		if err != nil {
			_, code := classify(err)
			return nil, safeProtocolError{err, code}
		}
		return conn, nil
	}
	opt := &redis.Options{Addr: b.Address, Username: b.Username, Password: password, DB: database, Protocol: 2, DisableIdentity: true, PoolSize: 1, MaxActiveConns: 1, MaxIdleConns: 1, MaxRetries: -1, DialerRetries: 1, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, ContextTimeoutEnabled: true, Dialer: safeDial}
	if b.TLS {
		host, _, _ := net.SplitHostPort(b.Address)
		cfg, e := tlsConfig(b.TLSCAFile, host)
		if e != nil {
			return e
		}
		opt.Dialer = func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, e := safeDial(ctx, network, address)
			if e != nil {
				return nil, e
			}
			secure := tls.Client(conn, cfg)
			if e = secure.HandshakeContext(ctx); e != nil {
				conn.Close()
				return nil, safeProtocolError{e, "tls_invalid"}
			}
			return secure, nil
		}
	}
	client := redis.NewClient(opt)
	defer client.Close()
	pong, e := client.Ping(ctx).Result()
	if e == nil && pong != "PONG" {
		return settings.Error("response_mismatch")
	}
	return e
}

// Driver pool logs must not print upstream addresses or raw TLS errors.
type safeProtocolError struct {
	cause error
	code  string
}

func (e safeProtocolError) Error() string { return e.code }
func (e safeProtocolError) Unwrap() error { return e.cause }
func (r *Runtime) clickhouse(ctx context.Context, b settings.Binding) error {
	cred, e := credential(b.CredentialFile, "password")
	if e != nil {
		return e
	}
	host, _, _ := net.SplitHostPort(b.Address)
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		conn, e := r.dial(ctx, "tcp", addr)
		if e != nil {
			return nil, e
		}
		if b.TLS {
			cfg, e := tlsConfig(b.TLSCAFile, host)
			if e != nil {
				conn.Close()
				return nil, e
			}
			secured := tls.Client(conn, cfg)
			if e = secured.HandshakeContext(ctx); e != nil {
				conn.Close()
				return nil, e
			}
			return secured, nil
		}
		return conn, nil
	}
	conn, e := clickhouse.Open(&clickhouse.Options{Addr: []string{b.Address}, Auth: clickhouse.Auth{Database: b.Database, Username: b.Username, Password: cred.Password}, DialContext: dial, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, MaxOpenConns: 1, MaxIdleConns: 0, Settings: clickhouse.Settings{"readonly": 1, "max_execution_time": 2}})
	if e != nil {
		return e
	}
	defer conn.Close()
	var one uint8
	e = conn.QueryRow(ctx, "SELECT 1").Scan(&one)
	if e == nil && one != 1 {
		return settings.Error("response_mismatch")
	}
	return e
}

type headOnly struct {
	transport http.RoundTripper
	host      string
	used      bool
	mu        chan struct{}
}

func (h *headOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	h.mu <- struct{}{}
	defer func() { <-h.mu }()
	if h.used || req.Method != http.MethodHead || req.URL.Host != h.host || req.URL.RawQuery != "" {
		return nil, settings.Error("permission_denied")
	}
	h.used = true
	return h.transport.RoundTrip(req)
}
func (r *Runtime) s3(ctx context.Context, b settings.Binding) error {
	cred, e := credential(b.CredentialFile, "s3")
	if e != nil {
		return e
	}
	u, e := settings.TargetURL(b.Endpoint, r.Bindings.AllowedTargetHosts)
	if e != nil {
		return e
	}
	client, e := r.httpClient(b.TLSCAFile, u.Hostname())
	if e != nil {
		return e
	}
	defer client.CloseIdleConnections()
	region := b.Region
	if region == "" {
		region = "us-east-1"
	}
	s3, e := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(cred.AccessKeyID, cred.SecretAccessKey, cred.SessionToken), Secure: u.Scheme == "https", Region: region, MaxRetries: 1, BucketLookup: minio.BucketLookupPath, Transport: &headOnly{transport: client.Transport, host: u.Host, mu: make(chan struct{}, 1)}})
	if e != nil {
		return e
	}
	exists, e := s3.BucketExists(ctx, b.Bucket)
	if e == nil && !exists {
		return settings.Error("response_mismatch")
	}
	return e
}
