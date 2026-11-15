// Package maintenance is the narrow public seam for offline diagnostics tools.
// It never starts a server or a business runtime; probes require explicit opt-in.
package maintenance

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/adapters"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"time"
)

type Inspection = store.Inspection

func Inspect(server string) (Inspection, error) {
	s, e := settings.LoadServer(server)
	if e != nil {
		return Inspection{}, e
	}
	return store.Inspect(s.StateDir, s.InstanceID, s.Limits)
}
func Compact(server string) error {
	s, e := settings.LoadServer(server)
	if e != nil {
		return e
	}
	return store.Compact(s.StateDir, s.InstanceID, s.Limits)
}

type ProbeResult struct {
	ProbeID    string `json:"probe_id"`
	Outcome    string `json:"outcome"`
	ReasonCode string `json:"reason_code"`
}

func Probe(ctx context.Context, server string) ([]ProbeResult, bool, error) {
	if e := settings.ValidateFiles(server); e != nil {
		return nil, false, e
	}
	s, _ := settings.LoadServer(server)
	results := []ProbeResult{}
	if s.LocalObserver == nil {
		return results, true, nil
	}
	var m settings.Manifest
	var b settings.Bindings
	if _, e := settings.Load(s.ManifestFile, &m); e != nil {
		return nil, false, e
	}
	if _, e := settings.Load(s.LocalObserver.BindingsFile, &b); e != nil {
		return nil, false, e
	}
	r := adapters.New(m, b, uuid.NewString())
	passed := true
	for _, p := range m.Probes {
		if p.NodeID != b.NodeID {
			continue
		}
		binding, _ := b.Find(p.BindingID)
		attempt, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMS)*time.Millisecond)
		o := r.Collect(attempt, p, binding)
		cancel()
		results = append(results, ProbeResult{p.ID, o.Outcome, o.ReasonCode})
		passed = passed && o.Outcome == "ok"
	}
	return results, passed, nil
}
