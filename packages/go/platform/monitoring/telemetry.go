package monitoring

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type traceKey struct{}
type traceMetrics struct {
	id                                                                 string
	started                                                            time.Time
	objectRead, objectWrite, objectErrors, objectReads, objectWrites   atomic.Int64
	cancelAt                                                           atomic.Int64
	stop                                                               func() bool
	inputRows, inputBytes, outputRows, outputBytes, heapPeak, tempPeak atomic.Uint64
	mu                                                                 sync.Mutex
	phase                                                              string
	phaseStarted                                                       time.Time
	phases                                                             map[string]int64
	states                                                             map[string]uint64
	identity                                                           map[string]string
}

// TraceContext owns one API call or leased job attempt. No source values,
// filenames, raw object paths, credentials or connection URLs enter its log.
func TraceContext(ctx context.Context, id string) context.Context {
	if _, e := uuid.Parse(id); e != nil {
		id = uuid.NewString()
	}
	m := &traceMetrics{id: id, started: time.Now(), phases: map[string]int64{}, states: map[string]uint64{}, identity: map[string]string{}}
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
	observeTaskMemory(ctx)
	args := []any{"trace_id", m.id, "operation", operation, "duration_ms", time.Since(m.started).Milliseconds(), "status", status, "response_bytes", responseBytes, "object_read_bytes", m.objectRead.Load(), "object_write_bytes", m.objectWrite.Load(), "object_reads", m.objectReads.Load(), "object_writes", m.objectWrites.Load(), "object_errors", m.objectErrors.Load(), "cancelled", cancelled}
	m.mu.Lock()
	if m.phase != "" {
		m.phases[m.phase] += time.Since(m.phaseStarted).Milliseconds()
		m.phase = ""
	}
	args = append(args, "input_rows", m.inputRows.Load(), "input_encoded_bytes", m.inputBytes.Load(), "computed_output_rows", m.outputRows.Load(), "output_encoded_bytes", m.outputBytes.Load(), "process_heap_peak_bytes", m.heapPeak.Load(), "temporary_peak_bytes", m.tempPeak.Load(), "phase_ms", m.phases, "calculation_states", m.states)
	for _, key := range []string{"task_id", "run_id", "generation_id", "source_position", "graph_revision", "config_revision", "fence", "error_code"} {
		if value := m.identity[key]; value != "" {
			args = append(args, key, value)
		}
	}
	if at := m.cancelAt.Load(); cancelled && at > 0 {
		args = append(args, "cancel_return_ms", time.Since(time.Unix(0, at)).Milliseconds())
	}
	slog.Info("monitoring.operation", args...)
	m.mu.Unlock()
}

// Counters describe physical execution work, including halo and rejected
// partitions. They are not unique logical input counts or billing quantities.
func RecordCalculationRead(ctx context.Context, count RawReadCount) {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		m.inputRows.Add(count.Rows)
		m.inputBytes.Add(count.Bytes)
	}
}

// Read stages accumulate independently of the surrounding task phase and use
// a fixed label set. They identify database qualification versus payload I/O.
func RecordCalculationReadDuration(ctx context.Context, stage string, elapsed time.Duration) {
	if !slices.Contains([]string{"read_qualify", "read_current", "read_payload", "read_recheck"}, stage) {
		return
	}
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		m.mu.Lock()
		m.phases[stage] += elapsed.Milliseconds()
		m.mu.Unlock()
	}
}
func RecordCalculationResult(ctx context.Context, state string, bytes uint64) {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		if !slices.Contains([]string{"valid", "baseline_invalid", "noncausal_baseline", "configuration_boundary", "invalid_input", "missing_input", "insufficient_samples", "gap_exceeded", "age_exceeded", "division_by_zero", "numeric_out_of_range", "numeric_underflow", "precision_unresolved"}, state) {
			state = "other"
		}
		m.outputRows.Add(1)
		m.outputBytes.Add(bytes)
		m.mu.Lock()
		m.states[state]++
		m.mu.Unlock()
	}
}
func RecordTaskIdentity(ctx context.Context, fields map[string]string) {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		m.mu.Lock()
		defer m.mu.Unlock()
		for key, value := range fields {
			if len(value) <= 128 && slices.Contains([]string{"task_id", "run_id", "generation_id", "source_position", "graph_revision", "config_revision", "fence", "error_code"}, key) {
				m.identity[key] = value
			}
		}
	}
}
func observeTaskMemory(ctx context.Context) {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		var v runtime.MemStats
		runtime.ReadMemStats(&v)
		recordPeak(&m.heapPeak, v.HeapAlloc)
	}
}
func recordPeak(counter *atomic.Uint64, value uint64) {
	for previous := counter.Load(); value > previous; previous = counter.Load() {
		if counter.CompareAndSwap(previous, value) {
			return
		}
	}
}
func RecordTemporaryPeak(ctx context.Context, bytes uint64) {
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		recordPeak(&m.tempPeak, bytes)
	}
}
func RecordTaskPhase(ctx context.Context, phase string) {
	recordTaskProgress(ctx, map[string]string{"phase": phase})
}
func recordTaskProgress(ctx context.Context, progress map[string]string) {
	observeTaskMemory(ctx)
	if m, ok := ctx.Value(traceKey{}).(*traceMetrics); ok {
		phase := progress["phase"]
		if phase != "" {
			if !slices.Contains([]string{"reading", "calculating", "sorting", "writing", "verifying", "publishing", "validating", "waiting_upstream", "exporting", "replaying", "completed"}, phase) {
				phase = "other"
			}
			m.mu.Lock()
			if phase != m.phase {
				if m.phase != "" {
					m.phases[m.phase] += time.Since(m.phaseStarted).Milliseconds()
				}
				m.phase, m.phaseStarted = phase, time.Now()
			}
			m.mu.Unlock()
		}
		if n, e := strconv.ParseUint(progress["temporary_peak_bytes"], 10, 64); e == nil {
			recordPeak(&m.tempPeak, n)
		}
	}
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
