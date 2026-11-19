// Package requestlimit bounds aggregate object transfers across a role's clients.
package requestlimit

import (
	"context"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
)

type Limit struct {
	slots chan struct{}
	used  atomic.Bool
}

func New(capacity int) *Limit {
	if capacity < 1 {
		return nil
	}
	return &Limit{slots: make(chan struct{}, capacity)}
}
func (l *Limit) Capacity() int {
	if l == nil {
		return 0
	}
	return cap(l.slots)
}

// Attached is true once a client transport actually enforces this limit.
func (l *Limit) Attached() bool { return l != nil && l.used.Load() }
func (l *Limit) Transport(base http.RoundTripper) http.RoundTripper {
	if l == nil {
		return base
	}
	l.used.Store(true)
	return transport{l, base}
}

type transport struct {
	limit *Limit
	base  http.RoundTripper
}

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	select {
	case t.limit.slots <- struct{}{}:
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
	release := sync.OnceFunc(func() { <-t.limit.slots })
	res, err := t.base.RoundTrip(r)
	if err != nil || res == nil || res.Body == nil || res.Body == http.NoBody {
		release()
		return res, err
	}
	raw := res.Body
	closeRaw := sync.OnceValue(raw.Close)
	stop := context.AfterFunc(r.Context(), func() {
		_ = closeRaw()
		release()
	})
	res.Body = &body{raw, closeRaw, func() { stop(); release() }}
	return res, nil
}

type body struct {
	io.Reader
	closeRaw func() error
	finish   func()
}

func (b *body) Read(dst []byte) (int, error) {
	n, err := b.Reader.Read(dst)
	if err != nil {
		b.finish()
	}
	return n, err
}
func (b *body) Close() error { defer b.finish(); return b.closeRaw() }
