package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"downaria-api/internal/core/config"
	"downaria-api/internal/shared/security"
	"go.uber.org/goleak"
)

func TestNoGoroutineLeak_HLSHandler(t *testing.T) {
	defer goleak.VerifyNone(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\n/seg.ts\n#EXT-X-ENDLIST\n"))
	}))
	defer up.Close()
	h := NewHandler(config.Config{Port: "8080", HLSStreamingEnabled: true}, time.Now())
	h.urlGuard = security.NewOutboundURLValidator(allowAllPublicResolver{})
	defer h.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-stream?url="+url.QueryEscape(up.URL+"/playlist.m3u8"), nil)
	rr := httptest.NewRecorder()
	h.HLSStream(rr, req)
}
