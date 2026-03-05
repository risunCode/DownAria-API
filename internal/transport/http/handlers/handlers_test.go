package handlers

import (
	"context"
	"encoding/json"
	"errors"
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
	"downaria-api/internal/shared/security"
)

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
			Port:                  "8080",
			AllowedOrigins:        []string{"http://localhost:3000"},
			GlobalRateLimitLimit:  60,
			GlobalRateLimitWindow: time.Minute,
			GlobalRateLimitRule:   "60/1m0s",
			UpstreamTimeout:       30 * time.Second,
			UpstreamTimeoutMS:     30000,
			MergeEnabled:          true,
			PublicBaseURL:         "http://localhost:8080",
			MaxDownloadSizeMB:     100,
		},
		startedAt:  time.Now(),
		httpClient: &http.Client{},
		statsStore: newMockStatsStore(),
		Streamer:   nil,
		extractor:  &mockExtractor{result: &core.ExtractResult{}},
		headCache:  nil,
		urlGuard:   security.NewOutboundURLValidator(allowAllPublicResolver{}),
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
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", data["status"])
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
