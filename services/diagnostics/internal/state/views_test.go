package state

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"testing"
	"time"
)

func snapshot() ingest.Snapshot {
	now := time.Now()
	boot := uuid.NewString()
	m := testfixture.Manifest()
	s := ingest.Snapshot{Manifest: m, BootID: boot, Now: now, Latest: map[string]model.StoredObservation{}, Online: map[string]bool{"node": true}}
	for _, p := range m.Probes {
		o := model.New(p, boot, now)
		o.Facets = model.Facets{Presence: "running", Readiness: "ready", Loop: "idle"}
		s.Latest[p.ID] = model.StoredObservation{Observation: o, NodeID: "node", ReceivedAt: now, FirstServerBootID: boot, ReceiptMonotonic: now, LastSuccessAt: o.ObservedAt, Persistence: "durable"}
	}
	return s
}
func TestKnownFailureWinsOverUnknownAndLastSuccessRemains(t *testing.T) {
	s := snapshot()
	old := s.Latest["report"]
	old.Outcome = "failed"
	old.Facets.Loop = "blocked"
	old.ReasonCode = "loop_failed"
	s.Latest["report"] = old
	delete(s.Latest, "presence")
	v := Build(s)
	r := v.Replicas["api-1"]
	if r.DisplayState != "unavailable" || r.Coverage.Unavailable != 1 || v.Components[0].FailedReplicas != 1 {
		t.Fatalf("failure masked %+v", r)
	}
	if v.Checks["report"].LastSuccessAt == nil {
		t.Fatal("last success lost")
	}
}
func TestStaleAndHistoricalNeverRemainReady(t *testing.T) {
	s := snapshot()
	if Build(s).Components[0].ReadyReplicas != 1 {
		t.Fatal("fresh idle not ready")
	}
	s.Now = s.Now.Add(21 * time.Second)
	v := Build(s)
	if v.Components[0].ReadyReplicas != 0 || v.Replicas["api-1"].Freshness != "stale" {
		t.Fatal("stale stayed ready")
	}
	s = snapshot()
	s.BootID = uuid.NewString()
	v = Build(s)
	if v.Replicas["api-1"].Freshness != "historical" || v.Components[0].ReadyReplicas != 0 {
		t.Fatal("old boot stayed ready")
	}
}
func TestReplicaDenominatorAndNonContinuousExpectations(t *testing.T) {
	s := snapshot()
	s.Manifest.Replicas = append(s.Manifest.Replicas, settings.Replica{ID: "api-2", ComponentID: "api", NodeID: "node", Expectation: "continuous", RequiredProbeIDs: []string{"missing"}})
	s.Manifest.Probes = append(s.Manifest.Probes, settings.Probe{ID: "missing", NodeID: "node", TargetKind: "replica", TargetID: "api-2", Adapter: "native_process", BindingID: "missing", Required: true, Facet: "presence", IntervalMS: 5000, TimeoutMS: 1000, StaleAfterMS: 20000})
	v := Build(s)
	c := v.Components[0]
	if c.ExpectedReplicas != 2 || c.ReadyReplicas != 1 || c.UnknownReplicas != 1 || c.DisplayState != "degraded" || !c.MinReadyMet {
		t.Fatalf("denominator %+v", c)
	}
	for _, expectation := range []string{"disabled", "on_demand", "external"} {
		s = snapshot()
		s.Manifest.Components[0].Expectation = expectation
		s.Manifest.Replicas[0].Expectation = expectation
		if expectation == "on_demand" {
			s.Latest = map[string]model.StoredObservation{}
		}
		v = Build(s)
		want := map[string]string{"disabled": "disabled", "on_demand": "idle", "external": "healthy"}[expectation]
		if v.Components[0].DisplayState != want || v.Components[0].ExpectedReplicas != 0 {
			t.Fatalf("%s %+v", expectation, v.Components[0])
		}
	}
}
