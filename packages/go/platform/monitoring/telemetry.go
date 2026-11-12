package monitoring

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type traceKey struct{}
type traceMetrics struct {
	id                                                               string
	started                                                          time.Time
	objectRead, objectWrite, objectErrors, objectReads, objectWrites atomic.Int64
	cancelAt                                                         atomic.Int64
	stop                                                             func() bool
}

// TraceContext owns one API call or leased job attempt. No source values,
// filenames, raw object paths, credentials or connection URLs enter its log.
func TraceContext(ctx context.Context, id string) context.Context {
	if _, e := uuid.Parse(id); e != nil {
		id = uuid.NewString()
	}
	m := &traceMetrics{id: id, started: time.Now()}
	m.stop = context.AfterFunc(ctx, func() { m.cancelAt.CompareAndSwap(0, time.Now().UnixNano()) })
	return context.WithValue(ctx, traceKey{}, m)
}
func TraceID(ctx context.Context) string {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		return m.id
	}
	return ""
}
func LogOperation(ctx context.Context, operation string, status int, responseBytes int64, cancelled bool) {
	m, ok := ctx.Value(traceKey{}).(*traceMetrics)
	if !ok {
		return
	}
	m.stop()
	args := []any{"trace_id", m.id, "operation", operation, "duration_ms", time.Since(m.started).Milliseconds(), "status", status, "response_bytes", responseBytes, "object_read_bytes", m.objectRead.Load(), "object_write_bytes", m.objectWrite.Load(), "object_reads", m.objectReads.Load(), "object_writes", m.objectWrites.Load(), "object_errors", m.objectErrors.Load(), "cancelled", cancelled}
	if at := m.cancelAt.Load(); cancelled && at > 0 {
		args = append(args, "cancel_return_ms", time.Since(time.Unix(0, at)).Milliseconds())
	}
	slog.Info("monitoring.operation", args...)
}
func chContext(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithQueryID("monitoring_"+TraceID(ctx)+"_"+uuid.NewString()))
}
func (s *Service) db(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if trace := TraceID(ctx); trace != "" {
			if _, e := tx.Exec(ctx, `SELECT set_config('infrapro.monitoring_trace_id',$1,true)`, trace); e != nil {
				return e
			}
		}
		return fn(tx)
	})
}

type objectReader interface {
	io.ReadCloser
	io.ReaderAt
}
type measuredObject struct {
	objectReader
	m *traceMetrics
}

func (r *measuredObject) Read(b []byte) (int, error) {
	n, e := r.objectReader.Read(b)
	r.m.objectRead.Add(int64(n))
	if e != nil && e != io.EOF {
		r.m.objectErrors.Add(1)
	}
	return n, e
}

func (r *measuredObject) ReadAt(b []byte, off int64) (int, error) {
	n, e := r.objectReader.ReadAt(b, off)
	r.m.objectRead.Add(int64(n))
	if e != nil && e != io.EOF {
		r.m.objectErrors.Add(1)
	}
	return n, e
}
