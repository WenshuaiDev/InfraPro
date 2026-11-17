package server

import (
	"crypto/tls"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/ddp"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/services/ingest-relay/internal/spool"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *Server) push(l *listener, w http.ResponseWriter, req *http.Request) {
	l.mu.RLock()
	src := l.source
	l.mu.RUnlock()
	p, e := s.Config.Principal(req.TLS, "producer")
	if e != nil || !slices.Contains(src.ProducerIDs, p.ID) {
		problem(w, 403, "source_credential_mismatch")
		return
	}
	if req.Method != "POST" || req.URL.Path != "/v1/push/"+src.SourceCode || req.URL.RawQuery != "" || req.Header.Get("Origin") != "" {
		problem(w, 403, "source_route_mismatch")
		return
	}
	key := req.Header.Get("Idempotency-Key")
	if len(key) < 1 || len(key) > 128 || strings.TrimSpace(key) != key {
		problem(w, 400, "idempotency_key_required")
		return
	}
	received := time.Now().UTC()
	b, e := io.ReadAll(io.LimitReader(req.Body, r.MaxBody+1))
	if e != nil || len(b) < 1 || len(b) > r.MaxBody {
		problem(w, 413, "limit_exceeded")
		return
	}
	current, authErr := s.Config.Principal(req.TLS, "producer")
	if authErr != nil || current.ID != p.ID {
		problem(w, 403, "source_credential_mismatch")
		return
	}
	out, replayed, e := s.Store.Receive(req.Context(), spool.Receive{Source: src, Principal: p.ID, DeliveryID: key, ReceivedAt: received, Raw: b})
	if e != nil {
		code := safeCode(e)
		status := 503
		if code == "idempotency_conflict" {
			status = 409
		}
		w.Header().Set("Retry-After", "5")
		problem(w, status, code)
		return
	}
	respond(w, 202, map[string]any{"receipt_id": out.ReceiptID, "spool_epoch": out.Epoch, "source_sequence": strconv.FormatInt(out.Sequence, 10), "sha256": out.SHA256, "replayed": replayed})
}
func (s *Server) accept(l *listener) {
	for {
		conn, e := l.socket.Accept()
		if e != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		l.mu.Lock()
		src := l.source
		l.connections[conn] = true
		l.mu.Unlock()
		go func() {
			defer func() { _ = conn.Close(); l.mu.Lock(); delete(l.connections, conn); l.mu.Unlock(); <-s.slots }()
			s.connection(conn, src)
		}()
	}
}
func (s *Server) connection(conn net.Conn, src r.Source) {
	t, ok := conn.(*tls.Conn)
	if !ok {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if t.HandshakeContext(s.ctx) != nil {
		return
	}
	state := t.ConnectionState()
	p, e := s.Config.Principal(&state, "producer")
	if e != nil || !slices.Contains(src.ProducerIDs, p.ID) {
		return
	}
	id := uuid.NewString()
	terminal := ""
	sequence := int64(0)
	defer func() {
		if terminal != "" {
			s.mu.Lock()
			if s.sessions[src.IngressID+"/"+terminal] == id {
				delete(s.sessions, src.IngressID+"/"+terminal)
			}
			s.mu.Unlock()
		}
	}()
	for {
		_ = conn.SetDeadline(time.Now().Add(time.Duration(src.IdleSeconds) * time.Second))
		b, e := ddp.Read(conn)
		sequence++
		if e != nil {
			if len(b) > 0 {
				s.fragment(src, p.ID, id, sequence, b, e.Error())
			}
			return
		}
		current, authErr := s.Config.Principal(&state, "producer")
		if authErr != nil || current.ID != p.ID || !slices.Contains(src.ProducerIDs, current.ID) {
			return
		}
		f, _ := ddp.Decode(b)
		if terminal == "" {
			if f.Type != 1 {
				s.fragment(src, p.ID, id, sequence, b, "registration_required")
				return
			}
			s.mu.Lock()
			key := src.IngressID + "/" + f.Terminal
			conflict := s.sessions[key] != ""
			if !conflict {
				s.sessions[key] = id
				terminal = f.Terminal
			}
			s.mu.Unlock()
			if conflict {
				s.fragment(src, p.ID, id, sequence, b, "active_session_conflict")
				return
			}
		} else if f.Terminal != terminal || f.Type == 1 {
			s.fragment(src, p.ID, id, sequence, b, "connection_identity_mismatch")
			return
		}
		if f.Type != 1 && f.Type != 2 && f.Type != 9 {
			s.fragment(src, p.ID, id, sequence, b, "unsupported_packet_type")
			return
		}
		_, _, e = s.Store.Receive(s.ctx, spool.Receive{Source: src, Principal: p.ID, ConnectionID: id, ConnectionSequence: sequence, ReceivedAt: time.Now().UTC(), Raw: b})
		if e != nil {
			return
		}
		ack, e := ddp.Acknowledgement(f)
		if e != nil {
			return
		}
		if _, e = conn.Write(ack); e != nil {
			return
		}
		if f.Type == 2 {
			return
		}
	}
}
func (s *Server) fragment(src r.Source, principal, id string, sequence int64, b []byte, code string) {
	truncated := len(b) > 8192
	if truncated {
		b = b[:8192]
	}
	// Only finite diagnostics enter logs/tables, never arbitrary exception text.
	if len(code) > 64 || strings.ContainsAny(code, " /:") {
		code = "connection_fragment"
	}
	_, _ = s.Store.Pool.Exec(s.ctx, `INSERT INTO relay_connection_events(id,ingress_id,connection_id,code,byte_length) SELECT $1,$2,$3,$4,$5 WHERE (SELECT count(*) FROM relay_connection_events WHERE ingress_id=$2 AND created_at>clock_timestamp()-interval '1 minute')<100`, uuid.NewString(), src.IngressID, id, code, len(b))
	_, _, _ = s.Store.Receive(s.ctx, spool.Receive{Source: src, Principal: principal, ConnectionID: id, ConnectionSequence: sequence, ReceivedAt: time.Now().UTC(), Raw: b, Invalid: true, Truncated: truncated})
}
