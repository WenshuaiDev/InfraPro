package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/eventbus"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"golang.org/x/sync/errgroup"
)

// Multiple keyed hints from one source can reach the same process together.
// Wait for the two real per-tenant domain slots locally before entering PG;
// database claims and executor leases still enforce quotas across processes.
type stageTenants struct {
	mu   sync.Mutex
	live map[string]*tenantCalls
}
type tenantCalls struct {
	slots      chan struct{}
	references int
}

func (s *stageTenants) run(ctx context.Context, tenant string, work func() error) error {
	s.mu.Lock()
	calls := s.live[tenant]
	if calls == nil {
		calls = &tenantCalls{slots: make(chan struct{}, 2)}
		s.live[tenant] = calls
	}
	calls.references++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		calls.references--
		if calls.references == 0 {
			delete(s.live, tenant)
		}
		s.mu.Unlock()
	}()
	select {
	case calls.slots <- struct{}{}:
		defer func() { <-calls.slots }()
		return work()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h host) ingestionConsumers(ctx context.Context, g *errgroup.Group) {
	s := h.b.Ingestion
	for _, name := range []string{"identify", "plan", "parse"} {
		g.Go(func() error {
			lanes := stageTenants{live: map[string]*tenantCalls{}}
			work := func(c context.Context, event eventbus.Event) error {
				state, e := s.AttemptDispatch(c, name, event.Scope, event.AggregateID, event.Generation)
				if e != nil {
					return e
				}
				if !state.Exists || state.Scope != event.Scope || state.Generation < event.Generation {
					return eventbus.ErrBadReference
				}
				if state.Generation == event.Generation && state.Pending {
					return eventbus.ErrDeferred
				}
				return nil
			}
			return eventbus.RunConsumer(ctx, h.resources.Pool, name, func(c context.Context, event eventbus.Event) error {
				if name == "plan" || event.Scope == "platform" {
					return work(c, event)
				}
				return lanes.run(c, event.Scope, func() error { return work(c, event) })
			}, h.report)
		})
	}
}
func (h host) changeConsumers(ctx context.Context, g *errgroup.Group, names ...string) {
	for _, name := range names {
		g.Go(func() error {
			return eventbus.RunConsumer(ctx, h.resources.Pool, name, func(c context.Context, e eventbus.Event) error {
				var err error
				switch name {
				case "summary":
					err = h.b.Monitoring.WakeSummaries(c, e.Scope, e.AggregateID, e.Generation)
				case "derived":
					if err = h.b.Monitoring.VerifyChange(c, e.Scope, e.AggregateID, e.Generation); err == nil {
						var reached bool
						reached, err = h.b.DerivedMonitoring.ConsumeThrough(c, e.Scope, e.Generation, time.Now())
						if err == nil && !reached {
							return eventbus.ErrDeferred
						}
					}
				default:
					err = h.b.Monitoring.ApplyCacheChange(c, name, e.Scope, e.AggregateID, e.Generation)
				}
				if errors.Is(err, monitoring.ErrMissingChange) {
					return eventbus.ErrBadReference
				}
				return err
			}, h.report)
		})
	}
}
