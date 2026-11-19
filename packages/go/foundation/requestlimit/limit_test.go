package requestlimit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSharedLimitRemainsHeldUntilBodyCloses(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "payload") }))
	defer s.Close()
	limit := New(1)
	first := &http.Client{Transport: limit.Transport(http.DefaultTransport)}
	second := &http.Client{Transport: limit.Transport(http.DefaultTransport)}
	a, err := first.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if res, err := second.Do(r); !errors.Is(err, context.DeadlineExceeded) {
		if res != nil {
			res.Body.Close()
		}
		t.Fatal(err)
	}
	if _, err = io.ReadAll(a.Body); err != nil {
		t.Fatal(err)
	}
	b, err := second.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.Body.Close()
	b.Body.Close()
}

func TestCancelReleasesUnreadBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "unread")
	}))
	defer server.Close()
	client := &http.Client{Transport: New(1).Transport(http.DefaultTransport), Timeout: time.Second}
	ctx, cancel := context.WithCancel(t.Context())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Body.Close()
	cancel()
	res, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}
