package diagnosticreport

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProcessMetricsRejectRawDimensionsAndBoundTenantQueues(t *testing.T) {
	r := newReporter(Report{}, func([]byte) error { return nil })
	defer r.Close()
	SetMetric(r, "event.consumer_position", 1<<54)
	Count(r, "event.publish_acks", math.MaxInt64)
	Count(r, "event.publish_acks", 3)
	Count(r, "event.publish_acks", -8)
	SetMetric(r, "event.raw_payload", 1)
	depths := map[string]int64{"tenant-email-private@example.test": 1, uuid.Nil.String(): 40}
	for i := 1; i <= 32; i++ {
		depths[uuid.NewString()] = int64(i)
	}
	SetTenantQueue(r, depths)
	raw := r.snapshot()
	for key := range depths {
		if strings.Contains(string(raw), key) {
			t.Fatal("raw dimension leaked")
		}
	}
	var report Report
	if e := json.Unmarshal(raw, &report); e != nil {
		t.Fatal(e)
	}
	if len(report.Metrics) != MaxQueueDimensions+2 {
		t.Fatal("wrong bounded metric count", len(report.Metrics))
	}
	if report.Metrics[0].Value != 1<<54 || report.Metrics[1].Value != math.MaxInt64 {
		t.Fatal("numeric precision or saturation lost")
	}
	for i, m := range report.Metrics[2:] {
		if !m.Valid() || m.Value != int64(32-i) || m.Key != TenantQueueKey {
			t.Fatal("queue selection", i, m)
		}
	}
	SetTenantQueue(r, nil)
	if strings.Contains(string(r.snapshot()), TenantQueueKey) {
		t.Fatal("drained tenant remained")
	}
	if (ProcessMetric{"event.publish_acks", "unexpected", 1}).Valid() || (ProcessMetric{TenantQueueKey, uuid.NewString(), 1}).Valid() {
		t.Fatal("unbounded dimension accepted")
	}
}
