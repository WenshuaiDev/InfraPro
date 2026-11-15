package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func TestOfflineCompactPreservesFencesAndRejectsLiveWriter(t *testing.T) {
	dir := testfixture.Directory(t)
	limits := settings.DefaultLimits()
	s, e := Open(dir, "lab", limits)
	if e != nil {
		t.Fatal(e)
	}
	f := Fence{NodeID: "remote", Generation: "7", Epoch: "42", BootID: "a", Sequence: "100"}
	if e = s.Commit(context.Background(), Change{Fences: []Fence{f}, Now: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if settings.Code(Compact(dir, "lab", limits)) != "file_locked" {
		t.Fatal("live writer compact allowed")
	}
	before, e := s.Load()
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Close()
	i, e := Inspect(dir, "lab", limits)
	if e != nil || len(i.Fences) != 1 || i.Fences[0].Sequence != "100" {
		t.Fatalf("inspect %v %+v", e, i)
	}
	if e = Compact(dir, "lab", limits); e != nil {
		t.Fatal(e)
	}
	s, e = Open(dir, "lab", limits)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	after, e := s.Load()
	if e != nil {
		t.Fatal(e)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("compaction changed logical data")
	}
}
func TestStorageBudgetStopsGrowthAndProtectsFence(t *testing.T) {
	dir := testfixture.Directory(t)
	limits := settings.DefaultLimits()
	limits.StoreMaxBytes = 1 << 20
	s, e := Open(dir, "lab", limits)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	f := Fence{NodeID: "remote", Generation: "1", Epoch: "2", Sequence: "8"}
	if e = s.Commit(context.Background(), Change{Fences: []Fence{f}, Now: now}); e != nil {
		t.Fatal(e)
	}
	maxSize := int64(0)
	for batch := 0; batch < 60; batch++ {
		metrics := []model.MetricPoint{}
		for j := 0; j < 100; j++ {
			metrics = append(metrics, model.MetricPoint{TargetKind: "node", TargetID: settings.NewID("n"), NodeID: "remote", Source: "os", Scope: "host", At: now, Metric: model.Metric{Key: "memory.total_bytes", Unit: "bytes", Value: "1099511627776", Availability: "available"}})
		}
		_ = s.Commit(context.Background(), Change{Metrics: metrics, Now: now})
		info, e := os.Stat(filepath.Join(dir, "state.db"))
		if e != nil {
			t.Fatal(e)
		}
		maxSize = max(maxSize, info.Size())
		if info.Size() > limits.StoreMaxBytes {
			t.Fatalf("budget exceeded: %d", info.Size())
		}
	}
	d, e := s.Load()
	if e != nil || d.Fences[Key("remote", "1")].Sequence != "8" {
		t.Fatal("fence lost at capacity")
	}
	t.Logf("injected 1 MiB limit; maximum physical size %d bytes, slack %d bytes", maxSize, limits.StoreMaxBytes-maxSize)
}
