package network

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConcurrentDownloader_DownloadPair(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	d := NewConcurrentDownloader(srv.Client())
	v, a, err := d.DownloadPair(t.Context(), srv.URL, srv.URL, nil)
	if err != nil {
		t.Fatalf("download pair failed: %v", err)
	}
	if v.Reader == nil || a.Reader == nil {
		t.Fatalf("expected both readers")
	}
	_ = v.Reader.Close()
	_ = a.Reader.Close()
}
