package diagnosticreport

import (
	"context"
	"time"
)

type deadlineKey struct{}
type deadlineSink interface{ SetDeadline(string, time.Time) }

// ObserveDeadlines associates an actual worker invocation with its lane. It
// changes no cancellation or deadline semantics of the owning operation.
func ObserveDeadlines(ctx context.Context, sink Sink, lane string) context.Context {
	s, ok := sink.(deadlineSink)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, deadlineKey{}, func(at time.Time) { s.SetDeadline(lane, at) })
}
func (g *Group) Context(ctx context.Context, worker int) context.Context {
	if g == nil || worker < 0 || worker >= len(g.workers) {
		return ctx
	}
	return context.WithValue(ctx, deadlineKey{}, func(at time.Time) { g.mu.Lock(); defer g.mu.Unlock(); g.workers[worker].deadline = at; g.publish() })
}
func TrackDeadline(parent, child context.Context) func() {
	notify, ok := parent.Value(deadlineKey{}).(func(time.Time))
	if !ok {
		return func() {}
	}
	notify(Deadline(child))
	return func() { notify(Deadline(parent)) }
}
func (r *Reporter) SetDeadline(id string, at time.Time) {
	r.lane(id, func(l *Lane) {
		if l.State == "working" {
			l.DeadlineAt = timeptr(at)
		}
	})
}
