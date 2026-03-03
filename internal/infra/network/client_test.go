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

	if transport.MaxIdleConns != 100 {
		t.Fatalf("MaxIdleConns=%d, want 100", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Fatalf("MaxIdleConnsPerHost=%d, want 10", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 20 {
		t.Fatalf("MaxConnsPerHost=%d, want 20", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout=%v, want 90s", transport.IdleConnTimeout)
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
