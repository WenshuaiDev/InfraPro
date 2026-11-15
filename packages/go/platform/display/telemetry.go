package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"sync"
	"time"
)

// RecordDiagnostic stores only bounded codes and correlation identifiers. A
// repeated failure is retained once per session/kind/minute, never per query.
func (s *Service) RecordDiagnostic(parent context.Context, v Session, kind, code string) {
	if !access.ValidID(v.ID) || len(code) > 80 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer cancel()
	_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('display-diagnostic:'||$1,0))`, v.ID); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO display_events(tenant_id,project_id,instance_id,kind,details) SELECT tenant_id,project_id,instance_id,$2,jsonb_build_object('session_id',id::text,'code',$3::text) FROM display_sessions s WHERE s.id=$1 AND NOT EXISTS(SELECT 1 FROM display_events e WHERE e.instance_id=s.instance_id AND e.kind=$2 AND e.details->>'session_id'=s.id::text AND e.created_at>clock_timestamp()-interval '1 minute')`, v.ID, kind, code)
		return e
	})
}

func (s *Service) StreamConnected(ctx context.Context, v Session) {
	var count int64
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE display_sessions SET stream_connections=stream_connections+1 WHERE id=$1 RETURNING stream_connections`, v.ID).Scan(&count)
	})
	if e == nil && count > 1 {
		s.Measure("reconnections", time.Now(), false)
	}
}

type measurement struct {
	Count    uint64 `json:"count"`
	Failures uint64 `json:"failures"`
	TotalMS  int64  `json:"total_ms"`
	MaxMS    int64  `json:"max_ms"`
}
type telemetry struct {
	sync.Mutex
	values map[string]measurement
}

func (s *Service) Measure(kind string, started time.Time, failed bool) {
	s.telemetry.Lock()
	defer s.telemetry.Unlock()
	if s.telemetry.values == nil {
		s.telemetry.values = map[string]measurement{}
	}
	m := s.telemetry.values[kind]
	m.Count++
	if failed {
		m.Failures++
	}
	ms := time.Since(started).Milliseconds()
	m.TotalMS += ms
	m.MaxMS = max(m.MaxMS, ms)
	s.telemetry.values[kind] = m
}

// Metrics returns process-local aggregate counters only, with no identities,
// query parameters, bearer values or business data. Counters reset on restart.
func (s *Service) Metrics() map[string]any {
	s.telemetry.Lock()
	values := map[string]measurement{}
	for k, v := range s.telemetry.values {
		values[k] = v
	}
	s.telemetry.Unlock()
	s.budget.mu.Lock()
	active := 0
	for k, v := range s.budget.active {
		if len(k) > 15 && k[:15] == "stream-session:" {
			active += v
		}
	}
	cache := len(s.budget.cache)
	denied, hits := s.budget.denied, s.budget.hits
	s.budget.mu.Unlock()
	return map[string]any{"measurements": values, "active_streams": active, "shared_queries": cache, "budget_rejections": denied, "query_cache_hits": hits}
}
func (s *Service) Readiness(ctx context.Context) error {
	if s.blobs == nil {
		return problem("dependency_unavailable", "包存储未配置。", 503)
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var generation string
		return tx.QueryRow(ctx, `SELECT generation::text FROM display_environment WHERE id=1`).Scan(&generation)
	})
}
