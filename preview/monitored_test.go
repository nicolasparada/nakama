package preview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMonitored_Fetch_OrderAndErrors(t *testing.T) {
	var hits int32
	// alternating success/failure endpoints
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`<!doctype html><html><head><meta property="og:title" content="OK"></head></html>`))
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "boom", 500)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := NewFetcher(32, time.Minute, 100*time.Millisecond, srv.Client())
	monitored := NewMonitored(f)
	defer monitored.Close()

	urls := []string{srv.URL + "/ok", srv.URL + "/boom", srv.URL + "/ok"}
	res := monitored.Fetch(context.TODO(), urls)
	if len(res) != len(urls) {
		t.Fatalf("len mismatch")
	}
	if res[0].Err != nil || res[0].Data.Title != "OK" {
		t.Fatalf("bad res[0]: %+v", res[0])
	}
	if res[1].Err == nil {
		t.Fatalf("expected error on res[1]")
	}
	if res[2].Err != nil || res[2].Data.Title != "OK" {
		t.Fatalf("bad res[2]: %+v", res[2])
	}
}
