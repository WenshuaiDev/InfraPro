// Package collector schedules fixed probes with bounded concurrency. Timeouts
// never free a slot until the underlying call actually returns, so a wedged
// adapter cannot create an unbounded goroutine retry loop.
package collector

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/adapters"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type result struct {
	id       string
	endpoint string
	version  uint64
	value    model.Observation
	at       time.Time
}
type slot struct {
	probe    settings.Probe
	binding  settings.Binding
	next     time.Time
	started  time.Time
	observed time.Time
	value    model.Observation
	active   bool
	timedOut bool
	failures int
	version  uint64
	cancel   context.CancelFunc
}
type Runner struct {
	mu        sync.Mutex
	slots     map[string]*slot
	adapter   adapters.Collector
	boot      string
	limit     int
	version   uint64
	results   chan result
	wake      chan struct{}
	active    int
	endpoints map[string]bool
	paused    bool
	now       func() time.Time
}

func New(boot string, limit int) *Runner {
	if limit < 1 || limit > 8 {
		limit = 8
	}
	return &Runner{slots: map[string]*slot{}, boot: boot, limit: limit, results: make(chan result, 8), wake: make(chan struct{}, 1), endpoints: map[string]bool{}, now: time.Now}
}

func (r *Runner) Boot(boot string) { r.mu.Lock(); r.boot = boot; r.mu.Unlock() }
func (r *Runner) Update(probes []settings.Probe, bindings settings.Bindings, adapter adapters.Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.version++
	r.adapter = adapter
	r.paused = false
	next := map[string]*slot{}
	for _, p := range probes {
		b, _ := bindings.Find(p.BindingID)
		s := &slot{probe: p, binding: b, value: model.Pending(p, r.boot), version: r.version}
		if old, ok := r.slots[p.ID]; ok && old.active {
			old.cancel()
			s.active = true
			s.started = old.started
			s.timedOut = old.timedOut
			s.cancel = old.cancel
		}
		next[p.ID] = s
	}
	for _, s := range r.slots {
		if s.active && s.cancel != nil {
			s.cancel()
		}
	}
	r.slots = next
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *Runner) Pause() {
	r.mu.Lock()
	r.paused = true
	for _, s := range r.slots {
		if s.active && s.cancel != nil {
			s.cancel()
		}
	}
	r.mu.Unlock()
}
func endpoint(p settings.Probe) string {
	if p.TargetKind == "endpoint" {
		return "endpoint:" + p.TargetID
	}
	return "probe:" + p.ID
}
func (r *Runner) Snapshot() []model.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Observation, 0, len(r.slots))
	now := r.now()
	for _, s := range r.slots {
		o := s.value
		if o.SampleAgeMS != nil {
			age := *o.SampleAgeMS + max(int64(0), now.Sub(s.observed).Milliseconds())
			o.SampleAgeMS = &age
		}
		out = append(out, o)
	}
	return out
}
func (r *Runner) Stats() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	blocked, delayed := 0, 0
	for _, s := range r.slots {
		if s.timedOut {
			blocked++
		}
		if !s.active && !s.next.IsZero() && r.now().Sub(s.next) > time.Duration(s.probe.IntervalMS)*time.Millisecond {
			delayed++
		}
	}
	return map[string]any{"assigned": len(r.slots), "active": r.active, "limit": r.limit, "blocked": blocked, "schedule_delayed": delayed, "paused": r.paused}
}
func (r *Runner) Run(ctx context.Context) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			r.Pause()
			return
		case result := <-r.results:
			r.complete(result)
		case <-tick.C:
		case <-r.wake:
		}
		r.schedule(ctx)
	}
}
func (r *Runner) schedule(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.paused || r.adapter == nil {
		return
	}
	now := r.now()
	for _, s := range r.slots {
		if s.active {
			if !s.timedOut && now.Sub(s.started) >= time.Duration(s.probe.TimeoutMS)*time.Millisecond {
				s.timedOut = true
				s.value = model.Failure(model.New(s.probe, r.boot, now), "timeout", "adapter_blocked")
				s.value.Facets = model.UnknownFacets()
				s.observed = now
			}
			continue
		}
		if now.Before(s.next) {
			continue
		}
		key := endpoint(s.probe)
		if r.active >= r.limit || r.endpoints[key] {
			continue
		}
		run, cancel := context.WithTimeout(ctx, time.Duration(s.probe.TimeoutMS)*time.Millisecond)
		s.cancel = cancel
		s.active = true
		s.started = now
		s.timedOut = false
		r.active++
		r.endpoints[key] = true
		p, b, a, version, boot := s.probe, s.binding, r.adapter, s.version, r.boot
		go func() {
			var o model.Observation
			func() {
				defer func() {
					if recover() != nil {
						o = model.Failure(model.New(p, boot, time.Now()), "failed", "adapter_error")
						o.Facets = model.UnknownFacets()
					}
				}()
				o = a.Collect(run, p, b)
			}()
			cancel()
			if model.Validate(o, p, boot) != nil {
				o = model.Failure(model.New(p, boot, time.Now()), "invalid_report", "report_invalid")
				o.Facets = model.UnknownFacets()
			}
			select {
			case r.results <- result{p.ID, key, version, o, time.Now()}:
			case <-ctx.Done():
			}
		}()
	}
}
func (r *Runner) complete(result result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = max(0, r.active-1)
	delete(r.endpoints, result.endpoint)
	s, ok := r.slots[result.id]
	if !ok {
		return
	}
	delete(r.endpoints, endpoint(s.probe))
	s.active = false
	s.cancel = nil
	s.timedOut = false
	if s.version != result.version {
		return
	}
	if result.at.Sub(s.started) > time.Duration(s.probe.TimeoutMS)*time.Millisecond {
		result.value = model.Failure(model.New(s.probe, r.boot, result.at), "timeout", "connection_timeout")
		result.value.Facets = model.UnknownFacets()
	}
	s.value = result.value
	s.observed = result.at
	delay := time.Duration(s.probe.IntervalMS) * time.Millisecond
	if result.value.Outcome != "ok" && result.value.Outcome != "pending" {
		s.failures++
		delay = max(delay, min(30*time.Second, time.Duration(1<<min(s.failures-1, 5))*time.Second))
		delay = time.Duration(float64(delay) * (0.9 + rand.Float64()*0.2))
	} else {
		s.failures = 0
	}
	s.next = result.at.Add(delay)
}
