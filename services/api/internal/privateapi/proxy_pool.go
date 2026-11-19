package privateapi

import (
	"io"
	"net/http"
	"sync"
	"time"
)

// A public request may already have committed when the owner connection fails.
// Even when a client supplies Idempotency-Key, the gateway must return that
// failure to the caller without silently issuing a second attempt.
type proxyPool struct {
	dial    *http.Transport
	address string
	active  chan struct{}
	mu      sync.Mutex
	closed  bool
	idle    []idleConnection
}

type idleConnection struct {
	conn  *http.ClientConn
	since time.Time
}

func newProxyPool(transport *http.Transport, address string, limit int) *proxyPool {
	// NewClientConn uses caller-managed capacity, separate from the Transport's
	// automatic per-host bookkeeping for normal RPCs.
	dial := transport.Clone()
	dial.MaxConnsPerHost = 0
	return &proxyPool{dial: dial, address: address, active: make(chan struct{}, limit)}
}

type streamLease struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *streamLease) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

func (p *proxyPool) take() (*http.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, http.ErrServerClosed
	}
	for len(p.idle) > 0 {
		last := len(p.idle) - 1
		slot := p.idle[last]
		p.idle = p.idle[:last]
		if slot.conn.Err() == nil && time.Since(slot.since) < p.dial.IdleConnTimeout {
			return slot.conn, nil
		}
		_ = slot.conn.Close()
	}
	return nil, nil
}

func (p *proxyPool) RoundTrip(r *http.Request) (*http.Response, error) {
	select {
	case <-r.Context().Done():
		return nil, r.Context().Err()
	case p.active <- struct{}{}:
	}
	conn, err := p.take()
	if err == nil && conn == nil {
		conn, err = p.dial.NewClientConn(r.Context(), "https", p.address)
	}
	if err != nil {
		<-p.active
		return nil, err
	}
	resp, err := conn.RoundTrip(r)
	if err != nil {
		_ = conn.Close()
		<-p.active
		return nil, err
	}
	resp.Body = &streamLease{ReadCloser: resp.Body, release: func() {
		p.mu.Lock()
		if p.closed || conn.Err() != nil {
			_ = conn.Close()
		} else {
			p.idle = append(p.idle, idleConnection{conn, time.Now()})
		}
		p.mu.Unlock()
		<-p.active
	}}
	return resp, nil
}

func (p *proxyPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, slot := range p.idle {
		_ = slot.conn.Close()
	}
	p.idle = nil
}
