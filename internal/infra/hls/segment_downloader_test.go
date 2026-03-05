package hls

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSegmentDownloader_ResolvesMultiLevelMasterToMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=3000\n/level1.m3u8\n"))
		case "/level1.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1200\n/media.m3u8\n"))
		case "/media.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\n/s0.ts\n#EXTINF:1.0,\n/s1.ts\n#EXT-X-ENDLIST\n"))
		case "/s0.ts":
			_, _ = w.Write([]byte("A"))
		case "/s1.ts":
			_, _ = w.Write([]byte("B"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	d := NewSegmentDownloader(srv.Client(), 4, 1)
	r, total, progress, err := d.DownloadAndConcatenate(t.Context(), srv.URL+"/root.m3u8", nil)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer r.Close()

	b, _ := io.ReadAll(r)
	if string(b) != "AB" {
		t.Fatalf("unexpected content: %q", string(b))
	}
	if total != 2 {
		t.Fatalf("unexpected total bytes: %d", total)
	}
	if progress.Progress() != 1 {
		t.Fatalf("expected full progress, got %f", progress.Progress())
	}
}

func TestSegmentDownloader_MasterVariantFailover(t *testing.T) {
	var firstVariantCalls atomic.Int64
	var secondVariantCalls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1500\n/a.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=1500\n/b.m3u8\n"))
		case "/a.m3u8":
			firstVariantCalls.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		case "/b.m3u8":
			secondVariantCalls.Add(1)
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\n/ok.ts\n#EXT-X-ENDLIST\n"))
		case "/ok.ts":
			_, _ = w.Write([]byte("Z"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	d := NewSegmentDownloader(srv.Client(), 2, 1)
	r, total, _, err := d.DownloadAndConcatenate(t.Context(), srv.URL+"/root.m3u8", nil)
	if err != nil {
		t.Fatalf("expected failover success, got error: %v", err)
	}
	defer r.Close()

	b, _ := io.ReadAll(r)
	if string(b) != "Z" || total != 1 {
		t.Fatalf("unexpected output after failover")
	}
	if firstVariantCalls.Load() == 0 {
		t.Fatalf("expected first variant attempt")
	}
	if secondVariantCalls.Load() == 0 {
		t.Fatalf("expected second variant attempt")
	}
}

func TestSegmentDownloader_MasterNoVariantsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/root.m3u8" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800\n\n"))
	}))
	defer srv.Close()

	d := NewSegmentDownloader(srv.Client(), 1, 1)
	_, _, _, err := d.DownloadAndConcatenate(t.Context(), srv.URL+"/root.m3u8", nil)
	if err == nil {
		t.Fatal("expected no-variant error")
	}
	if !strings.Contains(err.Error(), "master has no variants") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), srv.URL+"/root.m3u8") {
		t.Fatalf("error should include playlist URL: %v", err)
	}
}

func TestSegmentDownloader_InvalidPlaylistError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/broken.m3u8" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("this-is-not-a-playlist"))
	}))
	defer srv.Close()

	d := NewSegmentDownloader(srv.Client(), 1, 1)
	_, _, _, err := d.DownloadAndConcatenate(t.Context(), srv.URL+"/broken.m3u8", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse stage in error: %v", err)
	}
	if !strings.Contains(err.Error(), srv.URL+"/broken.m3u8") {
		t.Fatalf("error should include playlist URL: %v", err)
	}
}
