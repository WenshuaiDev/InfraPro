package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func TestAtomicFenceAndSnapshotCommit(t *testing.T) {
	dir := testfixture.Directory(t)
	s, e := Open(dir, "lab", settings.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	change := Change{Fences: []Fence{{NodeID: "node", Generation: "1", Epoch: "2", BootID: "boot", Sequence: "4"}}, Latest: []model.StoredObservation{{Observation: model.Observation{ProbeID: "probe"}}}, Now: time.Now()}
	s.beforeCommit = func() error { return errors.New("disk full") }
	if s.Commit(context.Background(), change) == nil {
		t.Fatal("acknowledged failed transaction")
	}
	d, e := s.Load()
	if e != nil || len(d.Fences) > 0 || len(d.Latest) > 0 {
		t.Fatal("partial commit")
	}
	s.beforeCommit = nil
	if e = s.Commit(context.Background(), change); e != nil {
		t.Fatal(e)
	}
	d, _ = s.Load()
	if len(d.Fences) != 1 || len(d.Latest) != 1 {
		t.Fatal("missing atomic values")
	}
	if second, e := Open(dir, "lab", settings.DefaultLimits()); e == nil {
		second.Close()
		t.Fatal("second writer")
	}
}
func TestWrongInstanceAndCorruptionPreserveOriginal(t *testing.T) {
	dir := testfixture.Directory(t)
	s, e := Open(dir, "lab", settings.DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	path := filepath.Join(dir, "state.db")
	before, _ := os.ReadFile(path)
	if other, e := Open(dir, "other", settings.DefaultLimits()); e == nil {
		other.Close()
		t.Fatal("wrong instance")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("wrong instance changed file")
	}
	broken := testfixture.Directory(t)
	path = filepath.Join(broken, "state.db")
	if e = os.WriteFile(path, []byte("broken-store-do-not-overwrite"), 0600); e != nil {
		t.Fatal(e)
	}
	if bad, e := Open(broken, "lab", settings.DefaultLimits()); e == nil {
		bad.Close()
		t.Fatal("corrupt database accepted")
	}
	after, _ = os.ReadFile(path)
	if string(after) != "broken-store-do-not-overwrite" {
		t.Fatal("corruption overwritten")
	}
}
func TestHistoryBudgetPreservesFences(t *testing.T) {
	limits := settings.DefaultLimits()
	limits.HistoryPoints = 2
	dir := testfixture.Directory(t)
	s, e := Open(dir, "lab", limits)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now := time.Now()
	m := []model.MetricPoint{}
	for i := 0; i < 4; i++ {
		m = append(m, model.MetricPoint{TargetKind: "node", TargetID: "node", At: now.Add(time.Duration(i) * time.Minute), Metric: model.Metric{Key: "cpu.utilization_ratio", Unit: "ratio", Value: 0.2, Availability: "available"}})
	}
	if e = s.Commit(context.Background(), Change{Metrics: m, Fences: []Fence{{NodeID: "node", Generation: "1", Epoch: "99", Sequence: "4"}}, Now: now}); e != nil {
		t.Fatal(e)
	}
	d, _ := s.Load()
	if len(d.Metrics) != 2 || d.Meta.EvictedCount != 2 || len(d.Fences) != 1 {
		t.Fatal("budget dropped fences or failed eviction")
	}
}
