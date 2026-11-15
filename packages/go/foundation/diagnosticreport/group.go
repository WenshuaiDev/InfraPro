package diagnosticreport

import (
	"sync"
	"time"
)

// Group keeps a fixed worker pool behind one public lane. Another worker's
// success cannot erase a blocked worker or its earliest real deadline.
type Group struct {
	mu      sync.Mutex
	sink    Sink
	id      string
	workers []workerState
}
type workerState struct {
	active         bool
	attempted      bool
	reason         string
	deadline, next time.Time
}

func NewGroup(sink Sink, id string, workers int) *Group {
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}
	return &Group{sink: OrNoop(sink), id: id, workers: make([]workerState, workers)}
}
func (g *Group) Begin(worker int, deadline time.Time) {
	if g == nil || worker < 0 || worker >= len(g.workers) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s := &g.workers[worker]
	s.active = true
	s.attempted = true
	s.reason = ""
	s.deadline = deadline
	s.next = time.Time{}
	g.publish()
}
func (g *Group) End(worker int, reason string, next time.Time) {
	if g == nil || worker < 0 || worker >= len(g.workers) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s := &g.workers[worker]
	s.active = false
	s.attempted = true
	s.reason = reason
	s.deadline = time.Time{}
	s.next = next
	g.publish()
}
func (g *Group) publish() {
	var deadline, next time.Time
	active := false
	reason := ""
	for _, s := range g.workers {
		if s.reason != "" {
			reason = s.reason
		}
		if s.active {
			active = true
			if !s.deadline.IsZero() && (deadline.IsZero() || s.deadline.Before(deadline)) {
				deadline = s.deadline
			}
		}
		if !s.next.IsZero() && (next.IsZero() || s.next.Before(next)) {
			next = s.next
		}
	}
	if reason != "" {
		g.sink.End(g.id, reason, next)
	} else if active {
		g.sink.Begin(g.id, deadline)
	} else {
		g.sink.End(g.id, "", next)
	}
}
func Optional(sinks []Sink) Sink {
	if len(sinks) > 0 {
		return OrNoop(sinks[0])
	}
	return OrNoop(nil)
}
