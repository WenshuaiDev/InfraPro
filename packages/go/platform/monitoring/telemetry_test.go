package monitoring

import (
	"context"
	"strconv"
	"testing"
)

func TestCalculationTelemetryBoundsLabelsAndCountsPhysicalWork(t *testing.T) {
	ctx := TraceContext(context.Background(), "")
	m := ctx.Value(traceKey{}).(*traceMetrics)
	defer m.stop()
	RecordCalculationRead(ctx, RawReadCount{Rows: 100, Bytes: 2048})
	RecordCalculationRead(ctx, RawReadCount{Rows: 25, Bytes: 512}) // repeated halo
	RecordCalculationResult(ctx, "valid", 500)
	RecordCalculationResult(ctx, "numeric_underflow", 400)
	for i := 0; i < 1000; i++ {
		RecordCalculationResult(ctx, "untrusted-"+strconv.Itoa(i), 0)
		RecordTaskPhase(ctx, "untrusted-"+strconv.Itoa(i))
	}
	RecordTaskIdentity(ctx, map[string]string{"task_id": "task", "object_key": "private", "password": "private"})
	RecordTemporaryPeak(ctx, 4096)
	RecordTemporaryPeak(ctx, 1024)
	if m.inputRows.Load() != 125 || m.inputBytes.Load() != 2560 || m.outputRows.Load() != 1002 || m.outputBytes.Load() != 900 {
		t.Fatal("physical work counters were conflated")
	}
	if len(m.states) != 3 || m.states["numeric_underflow"] != 1 || len(m.phases) > 1 || m.phase != "other" || len(m.identity) != 1 {
		t.Fatal("unbounded or private diagnostic label")
	}
	if m.tempPeak.Load() != 4096 || m.heapPeak.Load() == 0 {
		t.Fatal("resource observation lost its peak")
	}
}
