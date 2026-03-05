package cache

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeadDeduplicator_DeduplicatesConcurrentRequests(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewHeadDeduplicator(srv.Client(), 45*time.Second, 16)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.GetMetadata(t.Context(), srv.URL, nil)
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", hits.Load())
	}
}

func TestHeadDeduplicator_CacheTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewHeadDeduplicator(srv.Client(), 50*time.Millisecond, 16)
	_, _ = d.GetMetadata(t.Context(), srv.URL, nil)
	_, _ = d.GetMetadata(t.Context(), srv.URL, nil)
	if hits.Load() != 1 {
		t.Fatalf("expected cache hit")
	}
	time.Sleep(70 * time.Millisecond)
	_, _ = d.GetMetadata(t.Context(), srv.URL, nil)
	if hits.Load() != 2 {
		t.Fatalf("expected cache expiration")
	}
}

func BenchmarkHeadDeduplicator(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	d := NewHeadDeduplicator(srv.Client(), time.Second, 128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.GetMetadata(b.Context(), srv.URL, nil)
	}
}
