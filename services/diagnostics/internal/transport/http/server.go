package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/access"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/collector"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

const base = "/api/diagnostics/v1"

type Server struct {
	Config     settings.Server
	Access     *access.Manager
	Engine     *ingest.Engine
	Collector  *collector.Runner
	Started    time.Time
	querySlots chan struct{}
	snapshots  *snapshotCache
	rateMu     sync.Mutex
	rates      map[string]rateWindow
	// beforeWrite is used by the concurrent revocation regression test.
	beforeWrite func()
}
type rateWindow struct {
	at    time.Time
	count int
}

func New(cfg settings.Server, a *access.Manager, e *ingest.Engine, c *collector.Runner) *Server {
	return &Server{Config: cfg, Access: a, Engine: e, Collector: c, Started: time.Now(), querySlots: make(chan struct{}, 16), snapshots: newSnapshotCache(), rates: map[string]rateWindow{}}
}
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{Handler: s, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
}
func (s *Server) cookieName() string {
	if strings.HasPrefix(s.Config.PublicOrigin, "https://") {
		return "__Host-infrapro_diag"
	}
	return "infrapro_diag_local"
}
func (s *Server) setCookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(s.Config.PublicOrigin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func cookie(r *http.Request, name string) (string, error) {
	value := ""
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			value = c.Value
			count++
		}
	}
	if count > 1 {
		return "", settings.Error("authentication_required")
	}
	return value, nil
}
func (s *Server) origin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == s.Config.PublicOrigin || settings.OneOf(origin, s.Config.DevelopmentOrigins...)
}
func (s *Server) ip(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		return "unknown"
	}
	addr, e := netip.ParseAddr(host)
	if e != nil {
		return "unknown"
	}
	trusted := false
	for _, cidr := range s.Config.TrustedProxyCIDRs {
		prefix, e := netip.ParsePrefix(cidr)
		if e == nil && prefix.Contains(addr) {
			trusted = true
			break
		}
	}
	if trusted {
		forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if len(forwarded) == 1 {
			if ip, e := netip.ParseAddr(strings.TrimSpace(forwarded[0])); e == nil {
				return ip.String()
			}
		}
	}
	return addr.String()
}
func (s *Server) meta(id string) map[string]any {
	return map[string]any{"request_id": id, "server_time": time.Now().UTC().Truncate(time.Millisecond), "server_boot_id": s.Engine.BootID()}
}
func status(code string) int {
	switch code {
	case "invalid_request", "invalid_report", "protocol_unsupported":
		return 400
	case "authentication_required", "authentication_failed", "observer_session_expired":
		return 401
	case "permission_denied", "csrf_invalid", "origin_denied", "observer_scope_denied":
		return 403
	case "target_not_found", "target_removed":
		return 404
	case "snapshot_expired", "manifest_mismatch", "assignment_mismatch", "epoch_conflict", "stale_sequence", "sequence_conflict":
		return 409
	case "payload_too_large":
		return 413
	case "rate_limited":
		return 429
	case "auth_configuration_unavailable", "state_unavailable", "initialization_required", "keyring_invalid":
		return 503
	default:
		return 500
	}
}
func (s *Server) fail(w http.ResponseWriter, id string, err error) {
	code := settings.Code(err)
	if !settings.KnownReason(code) {
		code = "adapter_error"
	}
	retry := 0
	if code == "rate_limited" {
		w.Header().Set("Retry-After", "5")
		retry = 5000
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status(code))
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": s.meta(id), "error": map[string]any{"code": code, "message_key": code, "request_id": id, "retry_after_ms": retry}})
}
func (s *Server) write(w http.ResponseWriter, id string, data any, grant *access.Grant, meta map[string]any) {
	if meta == nil {
		meta = s.meta(id)
	}
	raw, e := json.Marshal(map[string]any{"meta": meta, "data": data})
	if e != nil || len(raw) > 1<<20 {
		s.fail(w, id, settings.Error("response_limit_exceeded"))
		return
	}
	if s.beforeWrite != nil {
		s.beforeWrite()
	}
	send := func() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}
	if grant == nil {
		send()
	} else if e = s.Access.Guard(*grant, send); e != nil {
		s.fail(w, id, e)
	}
}
func decode(w http.ResponseWriter, r *http.Request, max int64, dst any) error {
	if r.Header.Get("Content-Encoding") != "" {
		return settings.Error("invalid_request")
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return settings.Error("invalid_request")
	}
	body := http.MaxBytesReader(w, r.Body, max)
	defer body.Close()
	raw, e := io.ReadAll(body)
	var large *http.MaxBytesError
	if errors.As(e, &large) {
		return settings.Error("payload_too_large")
	}
	if e != nil || settings.Strict(raw, dst) != nil {
		return settings.Error("invalid_request")
	}
	return nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := uuid.NewString()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; connect-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if !settings.OneOf(r.Host, s.Config.AllowedHosts...) {
		s.fail(w, id, settings.Error("origin_denied"))
		return
	}
	if r.URL.RawQuery != "" && len(r.URL.RawQuery) > 4096 {
		s.fail(w, id, settings.Error("invalid_request"))
		return
	}
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
		return
	}
	if r.URL.Path == "/readyz" && r.Method == "GET" {
		_, allowed, _ := s.Access.State()
		view := s.Engine.Read()
		ready := allowed && web.Ready() && view.ConfigState == "ok" && view.StoreState == "ok"
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"status":"not_ready"}`)
		} else {
			_, _ = io.WriteString(w, `{"status":"ready"}`)
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/observer/") {
		s.observer(w, r, id)
		return
	}
	if strings.HasPrefix(r.URL.Path, base+"/session") {
		s.session(w, r, id)
		return
	}
	if strings.HasPrefix(r.URL.Path, base+"/") {
		if r.Method != "GET" {
			s.fail(w, id, settings.Error("permission_denied"))
			return
		}
		token, e := cookie(r, s.cookieName())
		if e != nil {
			s.fail(w, id, e)
			return
		}
		_, grant, e := s.Access.Current(token)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		if !s.Access.QueryAllowed(grant) {
			s.fail(w, id, settings.Error("rate_limited"))
			return
		}
		select {
		case s.querySlots <- struct{}{}:
			defer func() { <-s.querySlots }()
		default:
			s.fail(w, id, settings.Error("rate_limited"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		s.query(w, r.WithContext(ctx), id, grant)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.fail(w, id, settings.Error("target_not_found"))
		return
	}
	web.Handler().ServeHTTP(w, r)
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, id string) {
	path := strings.TrimPrefix(r.URL.Path, base)
	preName := s.cookieName() + "_preauth"
	if path == "/session/bootstrap" && r.Method == "GET" {
		if origin := r.Header.Get("Origin"); origin != "" && !s.origin(r) {
			s.fail(w, id, settings.Error("origin_denied"))
			return
		}
		nonce, csrf, initialized, allowed, e := s.Access.Bootstrap(s.ip(r))
		if e != nil {
			s.fail(w, id, e)
			return
		}
		s.setCookie(w, preName, nonce, 600)
		s.write(w, id, map[string]any{"initialized": initialized, "login_allowed": allowed, "csrf_token": csrf}, nil, nil)
		return
	}
	if path == "/session/login" && r.Method == "POST" {
		if !s.origin(r) {
			s.fail(w, id, settings.Error("origin_denied"))
			return
		}
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if e := decode(w, r, 4096, &in); e != nil {
			s.fail(w, id, e)
			return
		}
		if len(in.Username) > 64 || len(in.Password) > 128 {
			s.fail(w, id, settings.Error("authentication_failed"))
			return
		}
		nonce, e := cookie(r, preName)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		token, v, e := s.Access.Login(s.ip(r), nonce, r.Header.Get("X-CSRF-Token"), in.Username, in.Password)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		s.setCookie(w, s.cookieName(), token, 28800)
		s.setCookie(w, preName, "", -1)
		_, grant, e := s.Access.Current(token)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		s.write(w, id, v, &grant, nil)
		return
	}
	token, e := cookie(r, s.cookieName())
	if e != nil {
		s.fail(w, id, e)
		return
	}
	if path == "/session" && r.Method == "GET" {
		v, g, e := s.Access.Current(token)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		s.write(w, id, v, &g, nil)
		return
	}
	if r.Method == "POST" && (path == "/session/activity" || path == "/session/logout") {
		if !s.origin(r) {
			s.fail(w, id, settings.Error("origin_denied"))
			return
		}
		var body struct{}
		if e := decode(w, r, 4096, &body); e != nil {
			s.fail(w, id, e)
			return
		}
		if path == "/session/logout" {
			if e = s.Access.Logout(token, r.Header.Get("X-CSRF-Token")); e != nil {
				s.fail(w, id, e)
				return
			}
			s.setCookie(w, s.cookieName(), "", -1)
			w.WriteHeader(204)
			return
		}
		expires, e := s.Access.Activity(token, r.Header.Get("X-CSRF-Token"))
		if e != nil {
			s.fail(w, id, e)
			return
		}
		_, g, e := s.Access.Current(token)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		s.write(w, id, map[string]any{"idle_expires_at": expires}, &g, nil)
		return
	}
	s.fail(w, id, settings.Error("target_not_found"))
}
func (s *Server) allow(key string, limit int, period time.Duration) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	now := time.Now()
	for k, v := range s.rates {
		if now.Sub(v.at) >= time.Minute {
			delete(s.rates, k)
		}
	}
	v, ok := s.rates[key]
	if !ok || now.Sub(v.at) >= period {
		v = rateWindow{at: now}
	}
	if v.count >= limit || !ok && len(s.rates) >= 128 {
		return false
	}
	v.count++
	s.rates[key] = v
	return true
}
func bearer(r *http.Request) string {
	parts := strings.Split(r.Header.Get("Authorization"), " ")
	if len(parts) != 2 || parts[0] != "Bearer" || len(parts[1]) > 256 {
		return ""
	}
	return parts[1]
}
func (s *Server) observer(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" || r.Header.Get("Cookie") != "" || r.Header.Get("Content-Encoding") != "" {
		s.fail(w, id, settings.Error("observer_scope_denied"))
		return
	}
	token := bearer(r)
	if token == "" {
		s.fail(w, id, settings.Error("observer_scope_denied"))
		return
	}
	var data any
	switch r.URL.Path {
	case "/observer/v1/sessions":
		if !s.allow("handshake:ip:"+settings.Digest([]byte(s.ip(r))), 192, time.Minute) {
			s.fail(w, id, settings.Error("rate_limited"))
			return
		}
		node, keyErr := s.Engine.AuthenticateKey(token)
		if keyErr != nil {
			s.fail(w, id, keyErr)
			return
		}
		if !s.allow("handshake:node:"+node, 6, time.Minute) {
			s.fail(w, id, settings.Error("rate_limited"))
			return
		}
		var in ingest.SessionRequest
		if e := decode(w, r, 16<<10, &in); e != nil {
			s.fail(w, id, e)
			return
		}
		out, e := s.Engine.Handshake(r.Context(), token, in)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		data = out
	case "/observer/v1/reports":
		node, e := s.Engine.Authorize(token)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		if !s.allow("report:"+node, 2, time.Second) {
			s.fail(w, id, settings.Error("rate_limited"))
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			s.fail(w, id, settings.Error("invalid_request"))
			return
		}
		body := http.MaxBytesReader(w, r.Body, 1<<20)
		defer body.Close()
		raw, e := io.ReadAll(body)
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			s.fail(w, id, settings.Error("payload_too_large"))
			return
		}
		if e != nil {
			s.fail(w, id, settings.Error("invalid_report"))
			return
		}
		out, e := s.Engine.Accept(r.Context(), token, raw)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		data = out
	default:
		s.fail(w, id, settings.Error("target_not_found"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
