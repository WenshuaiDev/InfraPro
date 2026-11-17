package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/services/ingest-relay/internal/spool"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type listener struct {
	mu          sync.RWMutex
	source      r.Source
	socket      net.Listener
	http        *http.Server
	connections map[net.Conn]bool
}
type Server struct {
	Config    Config
	Store     *spool.Store
	mu        sync.Mutex
	listeners map[string]*listener
	sessions  map[string]string
	slots     chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

func New(cfg Config, store *spool.Store) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{Config: cfg, Store: store, listeners: map[string]*listener{}, sessions: map[string]string{}, slots: make(chan struct{}, cfg.MaxConnections), ctx: ctx, cancel: cancel}
}
func (s *Server) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.listeners {
		s.closeListener(l)
	}
}
func (s *Server) closeListener(l *listener) {
	_ = l.socket.Close()
	if l.http != nil {
		_ = l.http.Close()
	}
	l.mu.Lock()
	for conn := range l.connections {
		_ = conn.Close()
	}
	l.mu.Unlock()
}
func (s *Server) Restore(ctx context.Context) error {
	rows, e := s.Store.Pool.Query(ctx, `SELECT r.source FROM relay_listener_bindings b JOIN relay_source_revisions r ON r.ingress_id=b.ingress_id AND r.revision=b.revision WHERE b.active`)
	if e != nil {
		return e
	}
	list := []r.Source{}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return e
		}
		var src r.Source
		if e = json.Unmarshal(raw, &src); e != nil {
			rows.Close()
			return e
		}
		list = append(list, src)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, src := range list {
		if e = s.bind(src); e != nil {
			_, _ = s.Store.Pool.Exec(ctx, `UPDATE relay_listener_bindings SET state='failed',error_code='listener_bind_failed',observed_at=clock_timestamp() WHERE ingress_id=$1`, src.IngressID)
			return c.Fail("listener_bind_failed")
		}
	}
	return nil
}
func knownAdapter(src r.Source) bool {
	var want i.Adapter
	switch src.Transport {
	case "ddp_tcp":
		want = i.Adapter{ID: ddp.AdapterID, Version: ddp.AdapterVersion, ArtifactDigest: c.Digest([]byte(ddp.AdapterSource))}
	case "https_push":
		want = i.Adapter{ID: "https.raw-body", Version: "1.0.0", ArtifactDigest: c.Digest([]byte("https.raw-body/v1:mtls,4MiB,raw-body,source-scoped-idempotency"))}
	}
	return src.Adapter == want
}
func (s *Server) bind(src r.Source) error {
	if !knownAdapter(src) || src.RelayID != s.Config.RelayID || !slices.Contains(s.Config.ListenAddresses, src.Address) || src.Port < s.Config.PortMin || src.Port > s.Config.PortMax {
		return c.Fail("listener_configuration_invalid")
	}
	if old := s.listeners[src.IngressID]; old != nil {
		old.mu.RLock()
		previous := old.source
		old.mu.RUnlock()
		if !src.Active {
			s.closeListener(old)
			delete(s.listeners, src.IngressID)
			return nil
		}
		if previous.Address == src.Address && previous.Port == src.Port && previous.Transport == src.Transport {
			old.mu.Lock()
			old.source = src
			old.mu.Unlock()
			return nil
		}
		return c.Fail("pause_before_endpoint_change")
	}
	if !src.Active {
		return nil
	}
	tlsConfig, e := s.Config.TLS()
	if e != nil {
		return e
	}
	ln, e := tls.Listen("tcp", net.JoinHostPort(src.Address, strconv.Itoa(src.Port)), tlsConfig)
	if e != nil {
		return c.Fail("listener_bind_failed")
	}
	l := &listener{source: src, socket: ln, connections: map[net.Conn]bool{}}
	s.listeners[src.IngressID] = l
	if src.Transport == "https_push" {
		l.http = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { s.push(l, w, req) }), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, MaxHeaderBytes: 16 << 10}
		go func() { _ = l.http.Serve(ln) }()
	} else {
		go s.accept(l)
	}
	return nil
}
func (s *Server) Apply(ctx context.Context, in r.Apply) (r.Applied, error) {
	out := r.Applied{CommandID: in.CommandID, Digest: in.Source.Digest, ObservedAt: time.Now().UTC()}
	if _, e := uuid.Parse(in.CommandID); e != nil {
		return out, c.Fail("invalid_command")
	}
	if e := r.ValidateSource(in.Source); e != nil {
		return out, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(754041,hashtext(current_schema()))`); e != nil {
		return out, e
	}
	var hash string
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT request_digest,result FROM relay_command_receipts WHERE command_id=$1`, in.CommandID).Scan(&hash, &raw)
	if e == nil {
		if hash != r.Hash(in) {
			return out, c.Fail("command_conflict")
		}
		e = json.Unmarshal(raw, &out)
		return out, e
	}
	if e != pgx.ErrNoRows {
		return out, e
	}
	var generation int64
	var previous r.Source
	e = tx.QueryRow(ctx, `SELECT b.generation,r.source FROM relay_listener_bindings b JOIN relay_source_revisions r ON (b.ingress_id,b.revision)=(r.ingress_id,r.revision) WHERE b.ingress_id=$1`, in.Source.IngressID).Scan(&generation, &raw)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	if generation != in.ExpectedGeneration {
		return out, c.Fail("generation_conflict")
	}
	if e == nil {
		if json.Unmarshal(raw, &previous) != nil {
			return out, c.Fail("invalid_source")
		}
		if previous.TenantID != in.Source.TenantID || previous.ProjectID != in.Source.ProjectID || previous.ManufacturerID != in.Source.ManufacturerID || previous.ManufacturerScope != in.Source.ManufacturerScope || previous.NamespaceID != in.Source.NamespaceID || in.Source.Revision <= previous.Revision {
			return out, c.Fail("source_identity_immutable")
		}
	}
	rows, e := tx.Query(ctx, `SELECT r.source FROM relay_listener_bindings b JOIN relay_source_revisions r ON (b.ingress_id,b.revision)=(r.ingress_id,r.revision) WHERE b.active AND b.ingress_id<>$1`, in.Source.IngressID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return out, e
		}
		var other r.Source
		if json.Unmarshal(b, &other) != nil {
			rows.Close()
			return out, c.Fail("invalid_source")
		}
		if in.Source.Active && r.EndpointConflicts(in.Source, other) {
			rows.Close()
			return out, c.Fail("listener_conflict")
		}
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_source_revisions(ingress_id,revision,digest,source) VALUES($1,$2,$3,$4)`, in.Source.IngressID, in.Source.Revision, in.Source.Digest, spool.JSON(in.Source)); e != nil {
		return out, e
	}
	// Binding is local IO under the relay command lock, never a platform business
	// transaction. Only a successful bind is committed as active.
	bindErr := s.bind(in.Source)
	out.Generation = generation + 1
	out.State = "active"
	if !in.Source.Active {
		out.State = "paused"
	}
	if bindErr != nil {
		if old := s.listeners[in.Source.IngressID]; old != nil {
			s.closeListener(old)
			delete(s.listeners, in.Source.IngressID)
		}
		out.State = "failed"
		out.ErrorCode = "listener_bind_failed"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_listener_bindings(ingress_id,generation,revision,active,state,error_code) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(ingress_id) DO UPDATE SET generation=EXCLUDED.generation,revision=EXCLUDED.revision,active=EXCLUDED.active,state=EXCLUDED.state,error_code=EXCLUDED.error_code,observed_at=clock_timestamp()`, in.Source.IngressID, out.Generation, in.Source.Revision, in.Source.Active && bindErr == nil, out.State, out.ErrorCode); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_command_receipts(command_id,kind,request_digest,result) VALUES($1,'apply',$2,$3)`, in.CommandID, r.Hash(in), spool.JSON(out)); e != nil {
		return out, e
	}
	if e = tx.Commit(ctx); e != nil {
		if l := s.listeners[in.Source.IngressID]; l != nil {
			s.closeListener(l)
			delete(s.listeners, in.Source.IngressID)
		}
		return out, e
	}
	return out, nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code string) {
	respond(w, status, c.Error{Code: code, Message: code})
}
func decode(w http.ResponseWriter, req *http.Request, out any) bool {
	if req.Method != "POST" || req.URL.RawQuery != "" || req.Header.Get("Origin") != "" || strings.Split(req.Header.Get("Content-Type"), ";")[0] != "application/json" {
		problem(w, 400, "invalid_request")
		return false
	}
	b, e := io.ReadAll(io.LimitReader(req.Body, 1<<20+1))
	if e != nil || len(b) > 1<<20 || c.StrictJSON(b, out) != nil {
		problem(w, 400, "invalid_request")
		return false
	}
	return true
}
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Origin") != "" {
		problem(w, 403, "machine_only")
		return
	}
	p, e := s.Config.Principal(req.TLS, "consumer", "control")
	if e != nil {
		problem(w, 401, "machine_auth_failed")
		return
	}
	path := req.URL.Path
	if req.Method == "GET" && path == "/healthz" {
		respond(w, 200, map[string]string{"status": "ok"})
		return
	}
	if req.Method == "GET" && path == "/readyz" {
		if s.Store.Ready(req.Context()) != nil {
			problem(w, 503, "spool_not_durable")
		} else {
			respond(w, 200, map[string]string{"status": "ready"})
		}
		return
	}
	if req.Method == "GET" && path == "/v1/status" {
		s.status(w, req)
		return
	}
	if strings.HasPrefix(path, "/v1/control/") {
		if p.Role != "control" {
			problem(w, 403, "machine_role_denied")
			return
		}
		switch path {
		case "/v1/control/source-revisions":
			var in r.Apply
			if !decode(w, req, &in) {
				return
			}
			out, e := s.Apply(req.Context(), in)
			if e != nil {
				problem(w, 409, safeCode(e))
				return
			}
			respond(w, 200, out)
		case "/v1/control/source-watermarks":
			var in r.WatermarkRequest
			if !decode(w, req, &in) {
				return
			}
			out, e := s.Store.Watermark(req.Context(), in)
			if e != nil {
				problem(w, 409, safeCode(e))
				return
			}
			respond(w, 200, out)
		default:
			problem(w, 404, "not_found")
		}
		return
	}
	base := "/v1/consumers/" + p.ConsumerID
	if p.Role != "consumer" || p.ConsumerID == "" {
		problem(w, 403, "machine_role_denied")
		return
	}
	switch path {
	case base + "/pull":
		var in r.Pull
		if !decode(w, req, &in) {
			return
		}
		deadline := time.Now().Add(time.Duration(in.WaitSeconds) * time.Second)
		for {
			out, e := s.Store.Pull(req.Context(), in)
			if e != nil {
				problem(w, 503, safeCode(e))
				return
			}
			if len(out.Items) > 0 || !time.Now().Before(deadline) {
				respond(w, 200, out)
				return
			}
			select {
			case <-req.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	case base + "/acks":
		var in r.Acks
		if !decode(w, req, &in) {
			return
		}
		if len(in.Items) < 1 || len(in.Items) > 100 {
			problem(w, 400, "limit_exceeded")
			return
		}
		out := r.AckResults{Items: []r.AckResult{}}
		for _, v := range in.Items {
			result, e := s.Store.Ack(req.Context(), v)
			if e != nil {
				result.ErrorCode = safeCode(e)
			}
			out.Items = append(out.Items, result)
		}
		respond(w, 200, out)
	case base + "/releases":
		var in r.Releases
		if !decode(w, req, &in) {
			return
		}
		if len(in.Items) < 1 || len(in.Items) > 100 {
			problem(w, 400, "limit_exceeded")
			return
		}
		out := r.AckResults{Items: []r.AckResult{}}
		for _, v := range in.Items {
			result := r.AckResult{ReceiptID: v.ReceiptID, State: "released"}
			if e := s.Store.Release(req.Context(), v); e != nil {
				result.State = "rejected"
				result.ErrorCode = safeCode(e)
			}
			out.Items = append(out.Items, result)
		}
		respond(w, 200, out)
	default:
		problem(w, 404, "not_found")
	}
}
func safeCode(e error) string {
	var p *c.Error
	if errors.As(e, &p) {
		return p.Code
	}
	return "spool_unavailable"
}
func (s *Server) status(w http.ResponseWriter, req *http.Request) {
	var state map[string]any
	e := s.Store.Pool.QueryRow(req.Context(), `SELECT jsonb_build_object('relay_id',relay_id,'spool_epoch',spool_epoch,'used_bytes',used_bytes::text,'invalid_bytes',invalid_bytes::text,'budget_bytes',$1::bigint::text,'capacity_state',CASE WHEN used_bytes>=$1*9/10 THEN 'stopped' WHEN used_bytes>=$1*8/10 THEN 'warning' ELSE 'normal' END,'rejected_count',rejected_count::text,'last_rejected_at',last_rejected_at,'observed_at',clock_timestamp(),'pending',(SELECT count(*)::text FROM relay_deliveries WHERE state IN ('pending','leased')),'quarantined',(SELECT count(*)::text FROM relay_deliveries WHERE state='quarantined')) FROM relay_state WHERE id=1`, s.Store.Budget).Scan(&state)
	if e != nil {
		problem(w, 503, "spool_unavailable")
		return
	}
	rows, e := s.Store.Pool.Query(req.Context(), `SELECT b.ingress_id::text,jsonb_build_object('state',b.state,'revision',b.revision::text,'digest',r.digest,'error_code',b.error_code,'used_bytes',COALESCE(c.used_bytes,0)::text,'invalid_bytes',COALESCE(c.invalid_bytes,0)::text,'budget_bytes',r.source->>'budget_bytes','capacity_state',CASE WHEN COALESCE(c.used_bytes,0)>=(r.source->>'budget_bytes')::bigint*9/10 THEN 'stopped' WHEN COALESCE(c.used_bytes,0)>=(r.source->>'budget_bytes')::bigint*8/10 THEN 'warning' ELSE 'normal' END) FROM relay_listener_bindings b JOIN relay_source_revisions r ON r.ingress_id=b.ingress_id AND r.revision=b.revision LEFT JOIN LATERAL (SELECT sum(used_bytes) used_bytes,sum(invalid_bytes) invalid_bytes FROM relay_source_counters WHERE ingress_id=b.ingress_id) c ON true`)
	if e != nil {
		problem(w, 503, "spool_unavailable")
		return
	}
	sources := map[string]map[string]any{}
	for rows.Next() {
		var id string
		var v map[string]any
		if e = rows.Scan(&id, &v); e != nil {
			rows.Close()
			problem(w, 503, "spool_unavailable")
			return
		}
		sources[id] = v
	}
	rows.Close()
	if rows.Err() != nil {
		problem(w, 503, "spool_unavailable")
		return
	}
	s.mu.Lock()
	for id, v := range sources {
		n := 0
		transport := ""
		if l := s.listeners[id]; l != nil {
			l.mu.RLock()
			n = len(l.connections)
			transport = l.source.Transport
			l.mu.RUnlock()
		}
		v["connection_count"] = n
		v["connection_state"] = "disconnected"
		if transport == "https_push" {
			v["connection_state"] = "request_based"
		} else if n > 0 {
			v["connection_state"] = "connected"
		}
	}
	s.mu.Unlock()
	state["sources"] = sources
	respond(w, 200, state)
}
