package diagnosticreport

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Process metrics accept fixed technical names and nonnegative integers only.
// Trace IDs, subject identifiers and operation payloads belong to the owning
// domain; they cannot become labels or values in an observer report.
type ProcessMetric struct {
	Key       string `json:"key"`
	Dimension string `json:"dimension_id,omitempty"`
	Value     int64  `json:"value"`
}

const TenantQueueKey = "tenant.queue_depth"
const MaxQueueDimensions = 12
const MaxProcessMetrics = 48

var metricUnits = map[string]string{
	"event.outbox_pending": "count", "event.outbox_oldest_ms": "milliseconds", "event.publish_acks": "count",
	"event.delivery_lag_ms": "milliseconds", "event.redeliveries": "count", "event.quarantined": "count",
	"event.consumer_position": "position", "event.position_gaps": "count", "event.pg_reconciliations": "count",
	"dispatch.rejected_fences": "count", "parser.duration_ms": "milliseconds",
	"quota.pg_connections": "count", "quota.ch_connections": "count", "quota.s3_inflight": "count",
	"quota.parser_slots": "count", "quota.http_inflight": "count", TenantQueueKey: "count",
}
var queueDimension = regexp.MustCompile(`^tenant-[0-9a-f]{24}$`)

func MetricUnit(key string) string { return metricUnits[key] }
func MetricKeys() []string {
	keys := make([]string, 0, len(metricUnits))
	for key := range metricUnits {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
func (m ProcessMetric) Valid() bool {
	if m.Value < 0 || MetricUnit(m.Key) == "" {
		return false
	}
	if m.Key == TenantQueueKey {
		return queueDimension.MatchString(m.Dimension)
	}
	return m.Dimension == ""
}
func SetMetric(s Sink, key string, value int64) {
	if r, ok := s.(*Reporter); ok && r != nil && key != TenantQueueKey {
		r.metric(ProcessMetric{Key: key, Value: value}, false)
	}
}
func Count(s Sink, key string, delta int64) {
	if r, ok := s.(*Reporter); ok && r != nil && key != TenantQueueKey {
		r.metric(ProcessMetric{Key: key, Value: delta}, true)
	}
}
func (r *Reporter) metric(m ProcessMetric, add bool) {
	if !m.Valid() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, prev := range r.report.Metrics {
		if prev.Key != m.Key || prev.Dimension != m.Dimension {
			continue
		}
		if add {
			m.Value += min(prev.Value, math.MaxInt64-m.Value)
		}
		r.report.Metrics[i] = m
		r.signal()
		return
	}
	if len(r.report.Metrics) < MaxProcessMetrics {
		r.report.Metrics = append(r.report.Metrics, m)
		r.signal()
	}
}

// SetTenantQueue replaces the previous bounded set of tenant depths.
// A UUID is opaque but also appears in business links; only a one-way digest
// of its binary form is disclosed. Empty samples remove stale dimensions.
func SetTenantQueue(s Sink, depths map[string]int64) {
	r, ok := s.(*Reporter)
	if !ok || r == nil {
		return
	}
	items := make([]ProcessMetric, 0, len(depths))
	for id, count := range depths {
		v, err := uuid.Parse(id)
		if err != nil || v == uuid.Nil || id != strings.ToLower(v.String()) || count <= 0 {
			continue
		}
		sum := sha256.Sum256(v[:])
		items = append(items, ProcessMetric{TenantQueueKey, "tenant-" + hex.EncodeToString(sum[:12]), count})
	}
	slices.SortFunc(items, func(a, b ProcessMetric) int {
		if order := cmp.Compare(b.Value, a.Value); order != 0 {
			return order
		}
		return cmp.Compare(a.Dimension, b.Dimension)
	})
	r.mu.Lock()
	r.report.Metrics = slices.DeleteFunc(r.report.Metrics, func(m ProcessMetric) bool { return m.Key == TenantQueueKey })
	r.report.Metrics = append(r.report.Metrics, items[:min(len(items), MaxQueueDimensions)]...)
	r.mu.Unlock()
	r.signal()
}
