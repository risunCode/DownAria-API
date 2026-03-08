package network

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewHTTPClient_TransportConfig(t *testing.T) {
	client := NewHTTPClient(12)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	// Optimized connection pool settings
	if transport.MaxIdleConns != 200 {
		t.Fatalf("MaxIdleConns=%d, want 200", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 20 {
		t.Fatalf("MaxIdleConnsPerHost=%d, want 20", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 100 {
		t.Fatalf("MaxConnsPerHost=%d, want 100", transport.MaxConnsPerHost)
	}
	// Optimized: compression disabled to avoid double compression
	if !transport.DisableCompression {
		t.Fatalf("DisableCompression=false, want true")
	}
	// Optimized: larger buffers for better throughput
	if transport.WriteBufferSize != 256*1024 {
		t.Fatalf("WriteBufferSize=%d, want 262144", transport.WriteBufferSize)
	}
	if transport.ReadBufferSize != 256*1024 {
		t.Fatalf("ReadBufferSize=%d, want 262144", transport.ReadBufferSize)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout=%v, want 90s", transport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLSHandshakeTimeout=%v, want 10s", transport.TLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("ResponseHeaderTimeout=%v, want 30s", transport.ResponseHeaderTimeout)
	}
	if transport.DisableKeepAlives {
		t.Fatalf("DisableKeepAlives=true, want false")
	}
	if transport.DialContext == nil {
		t.Fatalf("DialContext is nil")
	}

	if client.Timeout != 12*time.Second {
		t.Fatalf("client.Timeout=%v, want 12s", client.Timeout)
	}
}

func TestNewHTTPClient_ReusesConnections(t *testing.T) {
	var newConns int64

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	srv.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt64(&newConns, 1)
		}
	}
	srv.Start()
	defer srv.Close()

	client := NewHTTPClient(5)

	for i := 0; i < 2; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}

	if got := atomic.LoadInt64(&newConns); got != 1 {
		t.Fatalf("expected 1 new connection, got %d", got)
	}
}

func TestNewHTTPClientWithOptions_AcceptsStreamingRequestTimeoutAndTransportOverrides(t *testing.T) {
	client := NewHTTPClientWithOptions(HTTPClientOptions{
		RequestTimeout:        0,
		DialTimeout:           2 * time.Second,
		KeepAliveTimeout:      3 * time.Second,
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       6 * time.Second,
	})

	if client.Timeout != 0 {
		t.Fatalf("client.Timeout=%v, want 0", client.Timeout)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	if transport.TLSHandshakeTimeout != 4*time.Second {
		t.Fatalf("TLSHandshakeTimeout=%v, want 4s", transport.TLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout != 5*time.Second {
		t.Fatalf("ResponseHeaderTimeout=%v, want 5s", transport.ResponseHeaderTimeout)
	}
	if transport.IdleConnTimeout != 6*time.Second {
		t.Fatalf("IdleConnTimeout=%v, want 6s", transport.IdleConnTimeout)
	}

}
