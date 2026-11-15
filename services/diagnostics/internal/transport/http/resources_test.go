package httptransport

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"net/url"
	"testing"
	"time"
)

func TestMetricHistoryGapsAndRemovedTarget(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Minute)
	snapshot := ingest.Snapshot{Now: now, Manifest: testfixture.Manifest()}
	from := now.Add(-4 * time.Minute)
	for _, at := range []time.Time{from, from.Add(2 * time.Minute)} {
		snapshot.Metrics = append(snapshot.Metrics, model.MetricPoint{TargetKind: "node", TargetID: "node", At: at, Scope: "host", Source: "os", Metric: model.Metric{Key: "cpu.utilization_ratio", Unit: "ratio", Availability: "available", Value: .5}})
	}
	q := url.Values{"target_kind": {"node"}, "target_id": {"node"}, "metric_key": {"cpu.utilization_ratio"}, "from": {from.Format(time.RFC3339)}, "to": {now.Format(time.RFC3339)}}
	result, err := metrics(snapshot, q)
	if err != nil {
		t.Fatal(err)
	}
	data := result.(map[string]any)
	if len(data["points"].([]model.MetricPoint)) != 2 || len(data["missing_intervals"].([]time.Time)) != 2 {
		t.Fatal("missing minutes were filled or lost")
	}
	q.Set("target_id", "removed")
	if _, err = metrics(snapshot, q); settings.Code(err) != "target_removed" {
		t.Fatal("removed target retained a live view", err)
	}
	q.Set("target_id", "node")
	q.Set("dimension_id", "unexpected")
	if _, err = metrics(snapshot, q); settings.Code(err) != "invalid_request" {
		t.Fatal("invalid metric dimension accepted", err)
	}
}

func TestResourceHintsRequireFreshKnownSamplesAndSustainedCPU(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	boot := uuid.NewString()
	p := settings.Probe{ID: "resources", NodeID: "node", Adapter: "host_resources", StaleAfterMS: 30000}
	o := model.New(p, boot, now)
	o.Metrics = []model.Metric{{Key: "memory.total_bytes", Unit: "bytes", Value: "1000", Availability: "available"}, {Key: "memory.available_bytes", Unit: "bytes", Value: "10", Availability: "available"}, {Key: "disk.total_bytes", Unit: "bytes", DimensionID: "state", Value: "1000", Availability: "available"}, {Key: "disk.available_bytes", Unit: "bytes", DimensionID: "state", Value: "10", Availability: "available"}}
	stored := model.StoredObservation{Observation: o, NodeID: "node", FirstServerBootID: boot, ReceiptMonotonic: now, ReceivedAt: now, Persistence: "durable"}
	snapshot := ingest.Snapshot{Now: now, BootID: boot, Manifest: settings.Manifest{Probes: []settings.Probe{p}}, Latest: map[string]model.StoredObservation{p.ID: stored}, Online: map[string]bool{"node": true}, CPUSamples: map[string][]model.MetricPoint{}}
	hints := settings.ResourceHints{Enabled: true, CPURatio: .9, CPUSamples: 3, MemoryAvailableRatio: .1, DiskAvailableRatio: .1, DiskAvailableBytes: "100"}
	if len(resourceWarnings(snapshot, hints)) != 2 {
		t.Fatal("known pressure missing")
	}
	for i := 0; i < 3; i++ {
		snapshot.CPUSamples[p.ID] = append(snapshot.CPUSamples[p.ID], model.MetricPoint{At: now.Add(time.Duration(i-2) * 10 * time.Second), Metric: model.Metric{Value: .95, Availability: "available"}})
	}
	if len(resourceWarnings(snapshot, hints)) != 3 {
		t.Fatal("sustained CPU missing")
	}
	snapshot.CPUSamples[p.ID][1].Value = nil
	snapshot.CPUSamples[p.ID][1].Availability = "unavailable"
	if len(resourceWarnings(snapshot, hints)) != 2 {
		t.Fatal("unknown CPU raised pressure")
	}
	for i := range stored.Metrics {
		stored.Metrics[i].Value = nil
		stored.Metrics[i].Availability = "unavailable"
	}
	snapshot.Latest[p.ID] = stored
	if len(resourceWarnings(snapshot, hints)) != 0 {
		t.Fatal("unknown capacity raised pressure")
	}
	snapshot.Latest[p.ID] = model.StoredObservation{Observation: o, FirstServerBootID: boot, ReceiptMonotonic: now.Add(-time.Minute), ReceivedAt: now.Add(-time.Minute)}
	snapshot.Now = now.Add(time.Minute)
	if len(resourceWarnings(snapshot, hints)) != 0 {
		t.Fatal("stale capacity raised pressure")
	}
}
