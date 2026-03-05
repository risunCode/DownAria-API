package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"downaria-api/internal/core/config"
	mergeinfra "downaria-api/internal/infra/merge"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/shared/security"
)

func TestIntegration_StreamingDownloadFlow(t *testing.T) {
	h := NewHandler(config.Config{Port: "8080", StreamingDownloadEnabled: true}, time.Now())
	h.urlGuard = security.NewOutboundURLValidator(allowAllPublicResolver{})
	h.Streamer = network.NewStreamerWithClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 256*1024)))}, nil
	})})
	defer h.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy?url="+url.QueryEscape("https://example.com/video.mp4"), nil)
	rr := httptest.NewRecorder()
	h.Proxy(rr, req)
	if rr.Code != http.StatusOK || rr.Body.Len() == 0 {
		t.Fatalf("unexpected proxy response: code=%d len=%d", rr.Code, rr.Body.Len())
	}
}

func TestIntegration_MergeFlowConcurrentDownloads(t *testing.T) {
	merger := mergeinfra.NewStreamingMerger("missing-ffmpeg-bin", 10*1024*1024)
	var out bytes.Buffer
	_, err := merger.MergeAndStream(t.Context(), &mergeinfra.MergeInput{VideoURL: "https://example.com/video.mp4", AudioURL: "https://example.com/audio.m4a"}, &out)
	if err == nil {
		t.Fatalf("expected ffmpeg-backed merge to fail when ffmpeg is unavailable")
	}
}

func TestIntegration_HLSStreamingFlow(t *testing.T) {
	h := NewHandler(config.Config{Port: "8080", HLSStreamingEnabled: true}, time.Now())
	h.urlGuard = security.NewOutboundURLValidator(allowAllPublicResolver{})
	h.Streamer = network.NewStreamerWithClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "master.m3u8") {
			body := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\n/variant.m3u8\n"
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/vnd.apple.mpegurl"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/vnd.apple.mpegurl"}}, Body: io.NopCloser(strings.NewReader("#EXTM3U\n#EXTINF:1.0,\n/seg.ts\n#EXT-X-ENDLIST\n"))}, nil
	})})
	defer h.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-stream?url="+url.QueryEscape("https://example.com/master.m3u8"), nil)
	rr := httptest.NewRecorder()
	h.HLSStream(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/api/v1/hls-stream?url=") {
		t.Fatalf("expected rewritten hls route")
	}
}

func TestIntegration_HLSMergeFlow(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v.m3u8", "/a.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1.0,\n/s0.ts\n#EXTINF:1.0,\n/s1.ts\n#EXT-X-ENDLIST\n"))
		case "/s0.ts":
			_, _ = w.Write([]byte("0"))
		case "/s1.ts":
			_, _ = w.Write([]byte("1"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer up.Close()

	merger := mergeinfra.NewStreamingMerger("missing-ffmpeg-bin", 10*1024*1024)
	_, err := merger.MergeAndStream(t.Context(), &mergeinfra.MergeInput{VideoURL: up.URL + "/v.m3u8", AudioURL: up.URL + "/a.m3u8"}, io.Discard)
	if err == nil {
		t.Fatalf("expected ffmpeg-backed hls merge to fail when ffmpeg is unavailable")
	}
}
