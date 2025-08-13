package preview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetcher_SuccessAndCache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
<meta property="og:title" content="Hello">
<meta property="og:description" content="World">
<meta property="og:image" content="https://example.com/a.jpg">
</head><body></body></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(32, 2*time.Minute, 10*time.Second, nil)

	ctx := context.Background()
	og, err := f.Get(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if og.Title != "Hello" || og.Description != "World" || len(og.Images) != 1 || og.Images[0].URL != "https://example.com/a.jpg" {
		t.Fatalf("unexpected og: %+v", og)
	}

	// Second call should hit success cache (no new server hits)
	_, err = f.Get(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Get 2: %v", err)
	}
	if c := atomic.LoadInt32(&hits); c != 1 {
		t.Fatalf("expected 1 hit, got %d", c)
	}
}

func TestFetcher_ErrorBackoff(t *testing.T) {
	var hits int32
	status := int32(500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if atomic.LoadInt32(&status) != 200 {
			http.Error(w, "boom", int(atomic.LoadInt32(&status)))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
<meta property="og:title" content="OK">
</head><body></body></html>`))
	}))
	defer srv.Close()

	f := NewFetcher(32, time.Minute, 150*time.Millisecond, nil)

	ctx := context.Background()
	// First call fails and should be recorded in error cache
	if _, err := f.Get(ctx, srv.URL); err == nil {
		t.Fatalf("expected error on first get")
	}
	if c := atomic.LoadInt32(&hits); c != 1 {
		t.Fatalf("expected 1 hit after failure, got %d", c)
	}

	// Immediate second call should be blocked by backoff (no new server hit)
	if _, err := f.Get(ctx, srv.URL); err == nil || err != ErrBackoff {
		t.Fatalf("expected ErrBackoff, got %v", err)
	}
	if c := atomic.LoadInt32(&hits); c != 1 {
		t.Fatalf("expected still 1 hit after backoff, got %d", c)
	}

	// After TTL expires, change to success and try again
	time.Sleep(200 * time.Millisecond)
	atomic.StoreInt32(&status, 200)
	og, err := f.Get(ctx, srv.URL)
	if err != nil {
		t.Fatalf("expected success after ttl, got %v", err)
	}
	if og.Title != "OK" {
		t.Fatalf("unexpected og after ttl: %+v", og)
	}
}
