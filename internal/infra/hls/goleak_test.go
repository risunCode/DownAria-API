package hls

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/goleak"
)

func TestNoGoroutineLeak_HLSSegmentDownloader(t *testing.T) {
	defer goleak.VerifyNone(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/playlist.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\n/s.ts\n#EXT-X-ENDLIST\n"))
		case "/s.ts":
			_, _ = w.Write([]byte("s"))
		}
	}))
	defer srv.Close()
	d := NewSegmentDownloader(srv.Client(), 2, 1)
	r, _, _, _ := d.DownloadAndConcatenate(t.Context(), srv.URL+"/playlist.m3u8", nil)
	if r != nil {
		_ = r.Close()
	}
}
