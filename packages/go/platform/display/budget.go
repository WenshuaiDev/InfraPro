package display

import (
	"context"
	"sync"
	"time"
)

type Budget struct {
	denied, hits uint64
	mu           sync.Mutex
	active       map[string]int
	cache        map[string]*cachedQuery
}
type cachedQuery struct {
	done   chan struct{}
	value  any
	err    error
	until  time.Time
	users  int
	cancel context.CancelFunc
}

func NewBudget() *Budget { return &Budget{active: map[string]int{}, cache: map[string]*cachedQuery{}} }

// Limits are taken atomically across tenant, project, instance, session and method.
func (b *Budget) Take(keys map[string]int) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, n := range keys {
		if n < 1 || b.active[k] >= n {
			b.denied++
			return nil, problem("budget_exceeded", "当前读取负载已达到限制，请稍后重试。", 429)
		}
	}
	for k := range keys {
		b.active[k]++
	}
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for k := range keys {
			b.active[k]--
			if b.active[k] == 0 {
				delete(b.active, k)
			}
		}
	}, nil
}

// Sharing is only allowed after the caller has independently revalidated its
// authority. The key includes owner, effective range, versions and environment.
func (b *Budget) Share(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	b.mu.Lock()
	now := time.Now()
	for k, v := range b.cache {
		if !v.until.IsZero() && now.After(v.until) {
			delete(b.cache, k)
		}
	}
	v := b.cache[key]
	if v != nil {
		b.hits++
	}
	if v == nil && len(b.cache) >= 1024 {
		b.mu.Unlock()
		return fn(ctx)
	}
	if v == nil {
		// A single consumer cancelling must not abort another consumer's read.
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		v = &cachedQuery{done: make(chan struct{}), cancel: cancel}
		b.cache[key] = v
		go func() {
			value, err := fn(shared)
			cancel()
			b.mu.Lock()
			defer b.mu.Unlock()
			v.value, v.err = value, err
			v.until = time.Now().Add(time.Second)
			if err != nil && b.cache[key] == v {
				delete(b.cache, key)
			}
			close(v.done)
		}()
	}
	v.users++
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		v.users--
		if v.users == 0 && v.until.IsZero() {
			v.cancel()
			if b.cache[key] == v {
				delete(b.cache, key)
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-v.done:
		return v.value, v.err
	}
}

func (s *Service) OpenStream(v Session) (func(), error) {
	return s.budget.Take(map[string]int{"stream-session:" + v.ID: 2, "stream-instance:" + v.InstanceID: s.options.MaxSessions, "stream-tenant:" + v.Tenant: s.options.MaxSessions * 2})
}
