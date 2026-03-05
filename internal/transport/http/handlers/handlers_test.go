package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"downaria-api/internal/app/services/extraction"
	"downaria-api/internal/core/config"
	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/core/ports"
	"downaria-api/internal/extractors/core"
	mergeinfra "downaria-api/internal/infra/merge"
	"downaria-api/internal/infra/metrics"
	"downaria-api/internal/shared/security"
	"downaria-api/pkg/ffmpeg"
)

type handlersFakeMergeRunner struct {
	mergeFn func(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error)
}

func (f *handlersFakeMergeRunner) StreamMerge(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error) {
	if f.mergeFn != nil {
		return f.mergeFn(ctx, opts)
	}
	return &ffmpeg.FFmpegResult{Stdout: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
}

type allowAllPublicResolver struct{}

func (allowAllPublicResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type mockStatsStore struct {
	visitors    map[string]bool
	extractions int64
	downloads   int64
	totalVisits int64
}

func newMockStatsStore() *mockStatsStore {
	return &mockStatsStore{
		visitors: make(map[string]bool),
	}
}

func (m *mockStatsStore) RecordVisitor(visitorKey string, now time.Time) {
	if !m.visitors[visitorKey] {
		m.visitors[visitorKey] = true
		m.totalVisits++
	}
}

func (m *mockStatsStore) RecordExtraction(now time.Time) {
	m.extractions++
}

func (m *mockStatsStore) RecordDownload(now time.Time) {
	m.downloads++
}

func (m *mockStatsStore) Snapshot(now time.Time) ports.StatsSnapshot {
	return ports.StatsSnapshot{
		TodayVisits:      int64(len(m.visitors)),
		TotalVisits:      m.totalVisits,
		TotalExtractions: m.extractions,
		TotalDownloads:   m.downloads,
	}
}

type mockExtractor struct {
	result *core.ExtractResult
	err    error
}

func (m *mockExtractor) Extract(ctx context.Context, input extraction.ExtractInput) (*core.ExtractResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return &core.ExtractResult{}, nil
}

func newTestHandler() *Handler {
	return &Handler{
		config: config.Config{
			Port:                            "8080",
			AllowedOrigins:                  []string{"http://localhost:3000"},
			GlobalRateLimitLimit:            60,
			GlobalRateLimitWindow:           time.Minute,
			GlobalRateLimitRule:             "60/1m0s",
			UpstreamTimeout:                 30 * time.Second,
			UpstreamTimeoutMS:               30000,
			UpstreamConnectTimeout:          3 * time.Second,
			UpstreamConnectTimeoutMS:        3000,
			UpstreamTLSHandshakeTimeout:     4 * time.Second,
			UpstreamTLSHandshakeTimeoutMS:   4000,
			UpstreamResponseHeaderTimeout:   5 * time.Second,
			UpstreamResponseHeaderTimeoutMS: 5000,
			UpstreamIdleConnTimeout:         6 * time.Second,
			UpstreamIdleConnTimeoutMS:       6000,
			UpstreamKeepAliveTimeout:        7 * time.Second,
			UpstreamKeepAliveTimeoutMS:      7000,
			MergeEnabled:                    true,
			PublicBaseURL:                   "http://localhost:8080",
			MaxDownloadSizeMB:               100,
		},
		startedAt:  time.Now(),
		httpClient: &http.Client{},
		statsStore: newMockStatsStore(),
		Streamer:   nil,
		extractor:  &mockExtractor{result: &core.ExtractResult{}},
		headCache:  nil,
		urlGuard:   security.NewOutboundURLValidator(allowAllPublicResolver{}),
		metrics:    metrics.NewContentDeliveryMetrics(),
	}
}

func TestHealthHandler(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("expected success to be true")
	}

	data := response["data"].(map[string]interface{})
	if data["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", data["status"])
	}

	if _, ok := data["timestamp"]; !ok {
		t.Error("expected timestamp field")
	}
}

func TestSettingsHandler(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()

	h.Settings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("expected success to be true")
	}

	data := response["data"].(map[string]interface{})

	if data["public_base_url"] != "http://localhost:8080" {
		t.Errorf("expected public_base_url 'http://localhost:8080', got %v", data["public_base_url"])
	}

	if data["merge_enabled"] != true {
		t.Errorf("expected merge_enabled true, got %v", data["merge_enabled"])
	}

	if data["upstream_timeout_ms"] != float64(30000) {
		t.Errorf("expected upstream_timeout_ms 30000, got %v", data["upstream_timeout_ms"])
	}
	if data["upstream_connect_timeout_ms"] != float64(3000) {
		t.Errorf("expected upstream_connect_timeout_ms 3000, got %v", data["upstream_connect_timeout_ms"])
	}
	if data["upstream_tls_handshake_timeout_ms"] != float64(4000) {
		t.Errorf("expected upstream_tls_handshake_timeout_ms 4000, got %v", data["upstream_tls_handshake_timeout_ms"])
	}
	if data["upstream_response_header_timeout_ms"] != float64(5000) {
		t.Errorf("expected upstream_response_header_timeout_ms 5000, got %v", data["upstream_response_header_timeout_ms"])
	}
	if data["upstream_idle_conn_timeout_ms"] != float64(6000) {
		t.Errorf("expected upstream_idle_conn_timeout_ms 6000, got %v", data["upstream_idle_conn_timeout_ms"])
	}
	if data["upstream_keepalive_timeout_ms"] != float64(7000) {
		t.Errorf("expected upstream_keepalive_timeout_ms 7000, got %v", data["upstream_keepalive_timeout_ms"])
	}

	if data["global_rate_limit_limit"] != float64(60) {
		t.Errorf("expected global_rate_limit_limit 60, got %v", data["global_rate_limit_limit"])
	}

	if data["global_rate_limit_window"] != "1m0s" {
		t.Errorf("expected global_rate_limit_window 1m0s, got %v", data["global_rate_limit_window"])
	}

	if data["global_rate_limit_rule"] != "60/1m0s" {
		t.Errorf("expected global_rate_limit_rule 60/1m0s, got %v", data["global_rate_limit_rule"])
	}

	if _, ok := data["allowed_origins"]; !ok {
		t.Error("expected allowed_origins field")
	}

	if data["max_download_size_mb"] != float64(100) {
		t.Errorf("expected max_download_size_mb 100, got %v", data["max_download_size_mb"])
	}
}

func TestPublicStatsHandler(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil)
	rr := httptest.NewRecorder()

	h.PublicStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !response["success"].(bool) {
		t.Error("expected success to be true")
	}

	data := response["data"].(map[string]interface{})

	if _, ok := data["todayVisits"]; !ok {
		t.Error("expected todayVisits field")
	}

	if _, ok := data["totalVisits"]; !ok {
		t.Error("expected totalVisits field")
	}

	if _, ok := data["totalExtractions"]; !ok {
		t.Error("expected totalExtractions field")
	}

	if _, ok := data["totalDownloads"]; !ok {
		t.Error("expected totalDownloads field")
	}

	// Verify visit was recorded
	mockStore := h.statsStore.(*mockStatsStore)
	if mockStore.totalVisits != 1 {
		t.Errorf("expected 1 visit recorded, got %d", mockStore.totalVisits)
	}
}

func TestExtractHandler_InvalidMethod(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/extract", nil)
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestExtractHandler_InvalidJSON(t *testing.T) {
	h := newTestHandler()

	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["success"].(bool) {
		t.Error("expected success to be false")
	}

	err := response["error"].(map[string]interface{})
	if err["code"] != "INVALID_JSON" {
		t.Errorf("expected error code INVALID_JSON, got %v", err["code"])
	}
	if err["category"] != string(apperrors.CategoryValidation) {
		t.Errorf("expected error category %s, got %v", apperrors.CategoryValidation, err["category"])
	}
}

func TestExtractHandler_MissingURL(t *testing.T) {
	h := newTestHandler()
	h.extractor = &mockExtractor{err: apperrors.ErrInvalidURL}

	body := strings.NewReader(`{"cookie":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestExtractHandler_UnsupportedPlatformIncludesCategory(t *testing.T) {
	h := newTestHandler()
	h.extractor = &mockExtractor{err: apperrors.ErrUnsupportedPlatform}

	body := strings.NewReader(`{"url":"https://example.com/video"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	errResp := payload["error"].(map[string]any)
	if errResp["code"] != apperrors.CodePlatformNotFound {
		t.Fatalf("expected code %s, got %v", apperrors.CodePlatformNotFound, errResp["code"])
	}
	if errResp["category"] != string(apperrors.CategoryNotFound) {
		t.Fatalf("expected category %s, got %v", apperrors.CategoryNotFound, errResp["category"])
	}
}

func TestExtractHandler_RateLimitIncludesRetryHeadersAndMetadata(t *testing.T) {
	h := newTestHandler()
	h.extractor = &mockExtractor{err: errors.New("HTTP 429: Too Many Requests")}

	body := strings.NewReader(`{"url":"https://example.com/video"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, rr.Code)
	}

	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatalf("expected Retry-After header")
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	errResp := payload["error"].(map[string]any)
	if errResp["category"] != string(apperrors.CategoryRateLimit) {
		t.Fatalf("expected category %s, got %v", apperrors.CategoryRateLimit, errResp["category"])
	}

	metadata, ok := errResp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object in error response")
	}
	if metadata["retryAfter"] == nil {
		t.Fatalf("expected retryAfter in metadata")
	}
	if metadata["resetAt"] == nil {
		t.Fatalf("expected resetAt in metadata")
	}
}

func TestExtractHandler_SuccessIncludesCookieSourceLane(t *testing.T) {
	h := newTestHandler()
	h.extractor = &mockExtractor{result: &core.ExtractResult{Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}}

	body := strings.NewReader(`{"url":"https://example.com/video","cookie":"sid=user"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Extract(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object")
	}
	if got := meta["cookieSource"]; got != "userProvided" {
		t.Fatalf("expected cookieSource=userProvided, got %v", got)
	}
	if got := meta["accessMode"]; got != "private" {
		t.Fatalf("expected accessMode=private, got %v", got)
	}
	if got := meta["publicContent"]; got != false {
		t.Fatalf("expected publicContent=false, got %v", got)
	}
}

func TestProxyHandler_MissingURL(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy", nil)
	rr := httptest.NewRecorder()

	h.Proxy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["success"].(bool) {
		t.Error("expected success to be false")
	}

	err := response["error"].(map[string]interface{})
	if err["code"] != "INVALID_URL" {
		t.Errorf("expected error code INVALID_URL, got %v", err["code"])
	}

	if !strings.Contains(err["message"].(string), "url") {
		t.Errorf("expected error message to contain 'url', got %v", err["message"])
	}
}

func TestMergeHandler_InvalidMethod(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/merge", nil)
	rr := httptest.NewRecorder()

	h.Merge(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestMergeHandler_InvalidJSON(t *testing.T) {
	h := newTestHandler()

	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merge", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Merge(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["success"].(bool) {
		t.Error("expected success to be false")
	}

	err := response["error"].(map[string]interface{})
	if err["code"] != "INVALID_JSON" {
		t.Errorf("expected error code INVALID_JSON, got %v", err["code"])
	}
}

func TestMergeHandler_Disabled(t *testing.T) {
	h := newTestHandler()
	h.config.MergeEnabled = false

	body := strings.NewReader(`{"url":"https://youtu.be/abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merge", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Merge(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestMergeHandler_QueueFullIncludesRetryAfterMetadata(t *testing.T) {
	h := newTestHandler()
	h.config.ConcurrentMergeEnabled = true
	h.mergePool = mergeinfra.NewMergeWorkerPool(1, 0, mergeinfra.NewStreamingMergerWithRunner(&handlersFakeMergeRunner{mergeFn: func(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error) {
		time.Sleep(2 * time.Second)
		return &ffmpeg.FFmpegResult{Stdout: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
	}}, 1024*1024))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.mergePool.Shutdown(ctx)
	}()

	fillJob := func() error {
		return h.mergePool.Submit(&mergeinfra.MergeJob{
			Ctx:      t.Context(),
			Input:    &mergeinfra.MergeInput{VideoURL: "https://example.com/video", AudioURL: "https://example.com/audio"},
			Output:   io.Discard,
			ResultCh: make(chan error, 1),
		})
	}
	seeded := false
	for i := 0; i < 20; i++ {
		if err := fillJob(); err == nil {
			seeded = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !seeded {
		t.Fatalf("failed to seed active merge job")
	}
	time.Sleep(20 * time.Millisecond)
	if err := fillJob(); !errors.Is(err, mergeinfra.ErrWorkerPoolQueueFull) {
		t.Fatalf("expected queue full when worker is occupied, got %v", err)
	}

	body := strings.NewReader(`{"videoUrl":"https://example.com/video","audioUrl":"https://example.com/audio"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merge", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Merge(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	errResp := payload["error"].(map[string]any)
	metadata := errResp["metadata"].(map[string]any)
	if metadata["retryAfter"] == nil || metadata["resetAt"] == nil {
		t.Fatalf("expected retry metadata")
	}
	if metadata["queueDepth"] == nil || metadata["queueCapacity"] == nil {
		t.Fatalf("expected queue metadata")
	}
}

func TestWriteFFmpegResultAsAttachment_StreamsWithoutTempBufferingContract(t *testing.T) {
	rr := httptest.NewRecorder()
	result := &ffmpeg.FFmpegResult{Stdout: io.NopCloser(bytes.NewReader([]byte("merged")))}

	written, err := writeFFmpegResultAsAttachment(rr, result, "sample.mp4", "video/mp4", 1024)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if written != int64(len("merged")) {
		t.Fatalf("expected %d bytes written, got %d", len("merged"), written)
	}
	if rr.Body.String() != "merged" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
	if rr.Header().Get("Content-Length") != "" {
		t.Fatalf("expected no pre-buffered Content-Length header")
	}
}
