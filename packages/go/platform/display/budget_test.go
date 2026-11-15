package display

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedReadSurvivesOneConsumerCancellation(t *testing.T) {
	b := NewBudget()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, finish := make(chan struct{}), make(chan struct{})
	var runs atomic.Int32
	fn := func(shared context.Context) (any, error) {
		runs.Add(1)
		close(started)
		select {
		case <-finish:
			return "value", nil
		case <-shared.Done():
			return nil, shared.Err()
		}
	}
	first := make(chan error, 1)
	go func() { _, e := b.Share(ctx, "same", fn); first <- e }()
	<-started
	second := make(chan error, 1)
	go func() {
		v, e := b.Share(context.Background(), "same", fn)
		if e == nil && v != "value" {
			e = errors.New("wrong value")
		}
		second <- e
	}()
	deadline := time.After(time.Second)
	for {
		b.mu.Lock()
		users := b.cache["same"].users
		b.mu.Unlock()
		if users == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second consumer did not join")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if e := <-first; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(finish)
	if e := <-second; e != nil {
		t.Fatal(e)
	}
	if runs.Load() != 1 {
		t.Fatal("equivalent calls were not shared")
	}
}
func TestLastConsumerCancelsSharedRead(t *testing.T) {
	b := NewBudget()
	ctx, cancel := context.WithCancel(context.Background())
	started, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		_, _ = b.Share(ctx, "one", func(shared context.Context) (any, error) {
			close(started)
			<-shared.Done()
			close(stopped)
			return nil, shared.Err()
		})
	}()
	<-started
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("orphan query kept running")
	}
}
