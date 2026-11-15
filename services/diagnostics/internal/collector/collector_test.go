package collector

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"sync/atomic"
	"testing"
	"time"
)

type adapterFunc func(context.Context, settings.Probe, settings.Binding) model.Observation

func (f adapterFunc) Collect(c context.Context, p settings.Probe, b settings.Binding) model.Observation {
	return f(c, p, b)
}
func TestHungProbeNeverReentersAndOtherProbeKeepsRunning(t *testing.T) {
	boot := uuid.NewString()
	r := New(boot, 2)
	m := testfixture.Manifest()
	probes := m.Probes
	for i := range probes {
		probes[i].IntervalMS = 1000
		probes[i].TimeoutMS = 100
	}
	gate := make(chan struct{})
	var hung, healthy atomic.Int32
	adapter := adapterFunc(func(ctx context.Context, p settings.Probe, b settings.Binding) model.Observation {
		if p.ID == "presence" {
			hung.Add(1)
			<-gate
		} else {
			healthy.Add(1)
		}
		return model.New(p, boot, time.Now())
	})
	r.Update(probes, settings.Bindings{}, adapter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)
	time.Sleep(2400 * time.Millisecond)
	if hung.Load() != 1 || healthy.Load() < 2 {
		t.Fatalf("hung=%d healthy=%d", hung.Load(), healthy.Load())
	}
	for _, o := range r.Snapshot() {
		if o.ProbeID == "presence" && o.ReasonCode != "adapter_blocked" {
			t.Fatal("blocked evidence absent")
		}
	}
	close(gate)
}
func TestAdapterPanicIsIsolated(t *testing.T) {
	boot := uuid.NewString()
	r := New(boot, 1)
	m := testfixture.Manifest()
	r.Update(m.Probes[:1], settings.Bindings{}, adapterFunc(func(context.Context, settings.Probe, settings.Binding) model.Observation { panic("SECRET") }))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)
	time.Sleep(300 * time.Millisecond)
	o := r.Snapshot()[0]
	if o.ReasonCode != "adapter_error" {
		t.Fatal(o.ReasonCode)
	}
}
