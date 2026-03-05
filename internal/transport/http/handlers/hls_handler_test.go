package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"downaria-api/internal/core/config"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/shared/security"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHLSUtils(t *testing.T) {
	if !isHLSPlaylist("https://a/b/index.m3u8", "") {
		t.Fatalf("expected hls detection")
	}
	if resolveURL("seg.ts", "https://a/b/master.m3u8") != "https://a/b/seg.ts" {
		t.Fatalf("unexpected resolve")
	}
	v := url.Values{}
	v.Set("chunk", "1")
	if !parseChunkParam(v) {
		t.Fatalf("expected chunk=true")
	}
}

func TestHandleHLSRequest_MediaRewritesToChunk(t *testing.T) {
	h := NewHandler(config.Config{Port: "8080", HLSStreamingEnabled: true}, time.Now())
	h.urlGuard = security.NewOutboundURLValidator(allowAllPublicResolver{})
	h.Streamer = network.NewStreamerWithClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:9.0,\nseg0.ts\n#EXT-X-ENDLIST\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/vnd.apple.mpegurl"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	defer h.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-stream?url="+url.QueryEscape("https://example.com/index.m3u8"), nil)
	rr := httptest.NewRecorder()
	h.HLSStream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "chunk=1") {
		t.Fatalf("expected rewritten chunk URLs, got %q", body)
	}
	if rr.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected no-cache for playlist")
	}
}

func TestHandleHLSRequest_ChunkStreamsDirect(t *testing.T) {
	h := NewHandler(config.Config{Port: "8080", HLSStreamingEnabled: true}, time.Now())
	h.urlGuard = security.NewOutboundURLValidator(allowAllPublicResolver{})
	h.Streamer = network.NewStreamerWithClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp2t"}}, Body: io.NopCloser(strings.NewReader("segment"))}, nil
	})})
	defer h.Close()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-stream?url="+url.QueryEscape("https://example.com/seg.ts")+"&chunk=1", nil)
	rr := httptest.NewRecorder()
	h.HLSStream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Cache-Control") == "" || !strings.Contains(rr.Header().Get("Cache-Control"), "86400") {
		t.Fatalf("expected segment cache header")
	}
	if rr.Body.String() != "segment" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
