package privateapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
)

func TestReplicaHTTPBudgetAcrossListeners(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	entered := make(chan struct{}, 5)
	release, stop := context.WithCancel(ctx)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hold" {
			entered <- struct{}{}
			select {
			case <-release.Done():
			case <-r.Context().Done():
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	fetch := func(ctx context.Context, address string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		if err == nil && resp.StatusCode != http.StatusNoContent {
			return errors.New("unexpected response")
		}
		return err
	}
	first, second := privateapi.LimitHandlers(3), privateapi.LimitHandlers(2)
	shell := httptest.NewServer(first(handler))
	defer shell.Close()
	assets := httptest.NewServer(first(handler))
	defer assets.Close()
	other := httptest.NewServer(second(handler))
	defer other.Close()
	defer stop()
	done := make(chan error, 5)
	for _, u := range []string{shell.URL, shell.URL, assets.URL, other.URL, other.URL} {
		go func() { done <- fetch(ctx, u+"/hold") }()
	}
	for range 5 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("five independently allocated requests could not run")
		}
	}
	for _, u := range []string{shell.URL, assets.URL, other.URL} {
		timeout, end := context.WithTimeout(ctx, 100*time.Millisecond)
		err := fetch(timeout, u)
		end()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("request exceeded its replica allocation", err)
		}
	}
	stop()
	for range 5 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []string{shell.URL, assets.URL, other.URL} {
		if err := fetch(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
}
