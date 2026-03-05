package network

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewOptimizedHTTPClient_Config(t *testing.T) {
	c := NewOptimizedHTTPClient(OptimizedClientOptions{})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected http transport")
	}
	if tr.MaxIdleConns != 100 || tr.MaxIdleConnsPerHost != 10 {
		t.Fatalf("unexpected pool settings: %d/%d", tr.MaxIdleConns, tr.MaxIdleConnsPerHost)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Fatalf("expected http2 enabled")
	}
}

func TestConnectionReuse_Basic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewOptimizedHTTPClient(OptimizedClientOptions{})
	for i := 0; i < 3; i++ {
		resp, err := c.Get(server.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp.Body.Close()
	}
}
