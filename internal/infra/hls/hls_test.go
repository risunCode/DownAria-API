package hls

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafov/m3u8"
)

func TestParserAndRewriter(t *testing.T) {
	content := []byte("#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:9.0,\nseg.ts\n#EXT-X-ENDLIST\n")
	p := NewParser()
	pl, lt, err := p.ParsePlaylist(content)
	if err != nil || lt != m3u8.MEDIA {
		t.Fatalf("parse failed: %v", err)
	}
	m := pl.(*m3u8.MediaPlaylist)
	RewriteMediaPlaylist(m, "https://x/y/index.m3u8", "/api/web/hls-stream")
	if !strings.Contains(m.Encode().String(), "chunk=1") {
		t.Fatalf("expected chunk rewrite")
	}
}

func TestRewriteMasterPlaylist_WithAlternatives(t *testing.T) {
	// Master playlist with audio alternatives shared across variants
	content := []byte(`#EXTM3U
#EXT-X-VERSION:6
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio-128k",NAME="Audio",URI="/audio/128k.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1000000,AUDIO="audio-128k"
/video/720p.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2000000,AUDIO="audio-128k"
/video/1080p.m3u8
`)

	p := NewParser()
	pl, lt, err := p.ParsePlaylist(content)
	if err != nil || lt != m3u8.MASTER {
		t.Fatalf("parse failed: %v", err)
	}

	master := pl.(*m3u8.MasterPlaylist)
	baseURL := "https://example.com/video/master.m3u8"
	routePrefix := "/api/web/hls-stream"

	// Rewrite the playlist
	RewriteMasterPlaylist(master, baseURL, routePrefix)

	encoded := master.Encode().String()

	// Verify variant URIs are rewritten
	if !strings.Contains(encoded, "/api/web/hls-stream?url=https%3A%2F%2Fexample.com%2Fvideo%2F720p.m3u8") {
		t.Errorf("720p variant URI not correctly rewritten")
	}
	if !strings.Contains(encoded, "/api/web/hls-stream?url=https%3A%2F%2Fexample.com%2Fvideo%2F1080p.m3u8") {
		t.Errorf("1080p variant URI not correctly rewritten")
	}

	// Verify audio alternative URI is rewritten (should appear only once in EXT-X-MEDIA tag)
	expectedAudioURI := "/api/web/hls-stream?url=https%3A%2F%2Fexample.com%2Faudio%2F128k.m3u8"
	if !strings.Contains(encoded, expectedAudioURI) {
		t.Errorf("audio URI not correctly rewritten")
	}

	// Verify no recursive rewriting (should not contain nested proxy URLs)
	if strings.Contains(encoded, "hls-stream%3Furl%3Dhttps%253A%252F%252F") {
		t.Errorf("detected recursive URL encoding in output")
	}

	// Verify the audio URI appears exactly once (not duplicated due to shared Alternative pointers)
	count := strings.Count(encoded, expectedAudioURI)
	if count != 1 {
		t.Errorf("expected audio URI to appear exactly once, got %d times", count)
	}
}


func TestSegmentDownloader_DownloadAndConcatenate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
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

	d := NewSegmentDownloader(srv.Client(), 5, 1)
	r, total, progress, err := d.DownloadAndConcatenate(t.Context(), srv.URL+"/playlist.m3u8", nil)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer r.Close()
	b, _ := io.ReadAll(r)
	if string(b) != "AB" || total != 2 || progress.Progress() != 1 {
		t.Fatalf("unexpected concat output")
	}
}

func BenchmarkSegmentPool(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 32*1024))
	}))
	defer srv.Close()
	p := NewSegmentWorkerPool(srv.Client(), 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.FetchSegment(b.Context(), srv.URL, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestSegmentDownloader_Retry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	d := NewSegmentDownloader(srv.Client(), 1, 3)
	res := d.downloadSegmentWithRetry(t.Context(), srv.URL, nil, 0)
	if res.Error != nil {
		t.Fatalf("expected retry success: %v", res.Error)
	}
	if res.Attempts < 2 {
		t.Fatalf("expected multiple attempts")
	}
	_ = time.Millisecond
}
