//go:build integration

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"downaria-api/internal/app/services/extraction"
	"downaria-api/internal/core/config"
	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/core/ports"
	"downaria-api/internal/extractors/core"
	"downaria-api/internal/extractors/registry"
	"downaria-api/internal/infra/cache"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/shared/security"
)

type integrationResolver struct{}

func (integrationResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

type integrationStatsStore struct{}

func (integrationStatsStore) RecordVisitor(string, time.Time) {}

func (integrationStatsStore) RecordExtraction(time.Time) {}

func (integrationStatsStore) RecordDownload(time.Time) {}

func (integrationStatsStore) Snapshot(time.Time) ports.StatsSnapshot {
	return ports.StatsSnapshot{}
}

type scriptedExtractor struct {
	result *core.ExtractResult
	err    error
	failN  int
	calls  int
}

func (e *scriptedExtractor) Match(string) bool {
	return true
}

func (e *scriptedExtractor) Extract(url string, _ core.ExtractOptions) (*core.ExtractResult, error) {
	e.calls++
	if e.failN > 0 && e.calls <= e.failN {
		return nil, e.err
	}
	if e.err != nil && e.failN == 0 {
		return nil, e.err
	}
	if e.result == nil {
		return &core.ExtractResult{URL: url}, nil
	}
	cloned := *e.result
	if cloned.URL == "" {
		cloned.URL = url
	}
	return &cloned, nil
}

func newIntegrationHandler(t *testing.T, extractorSvc extraction.Service) *Handler {
	t.Helper()

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
		httpClient: network.GetClientWithTimeout(30 * time.Second),
		statsStore: integrationStatsStore{},
		Streamer:   network.NewStreamer(),
		extractor:  extractorSvc,
		headCache:  cache.NewTTLCache(),
		urlGuard:   security.NewOutboundURLValidator(integrationResolver{}),
	}
}

func registerMockService(t *testing.T, platform string, re *regexp.Regexp, ext core.Extractor, retries int) extraction.Service {
	t.Helper()

	reg := registry.NewRegistry()
	reg.Register(platform, []*regexp.Regexp{re}, func() core.Extractor { return ext })
	return extraction.NewService(reg, 30, retries, 0)
}

func postExtract(t *testing.T, h *Handler, url string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"url": url})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Extract(rr, req)
	return rr
}

func decodeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return payload
}

func TestExtractIntegration_SuccessFlow(t *testing.T) {
	ext := &scriptedExtractor{result: &core.ExtractResult{}}
	svc := registerMockService(t, "mockplatform", regexp.MustCompile(`^https://integration-success\.example/.*$`), ext, 3)
	h := newIntegrationHandler(t, svc)

	rr := postExtract(t, h, "https://integration-success.example/video/123")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	payload := decodeEnvelope(t, rr)
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected success=true, got payload=%v", payload)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if data["platform"] != "mockplatform" {
		t.Fatalf("expected platform mockplatform, got %v", data["platform"])
	}
	if ext.calls != 1 {
		t.Fatalf("expected extractor to be called once, got %d", ext.calls)
	}
}

func TestExtractIntegration_TransientRetryThenSuccess(t *testing.T) {
	ext := &scriptedExtractor{
		result: &core.ExtractResult{},
		err:    &net.DNSError{Err: "temporary", Name: "integration.example", IsTemporary: true},
		failN:  2,
	}
	svc := registerMockService(t, "retryplatform", regexp.MustCompile(`^https://integration-retry\.example/.*$`), ext, 3)
	h := newIntegrationHandler(t, svc)

	rr := postExtract(t, h, "https://integration-retry.example/video/123")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if ext.calls != 3 {
		t.Fatalf("expected extractor calls=3 (2 retries + success), got %d", ext.calls)
	}
}

func TestExtractIntegration_PermanentErrorNoRetry(t *testing.T) {
	ext := &scriptedExtractor{err: fmt.Errorf("HTTP 401: Unauthorized")}
	svc := registerMockService(t, "authplatform", regexp.MustCompile(`^https://integration-auth\.example/.*$`), ext, 5)
	h := newIntegrationHandler(t, svc)

	rr := postExtract(t, h, "https://integration-auth.example/video/123")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if ext.calls != 1 {
		t.Fatalf("expected no retry for permanent auth error; calls=%d", ext.calls)
	}

	payload := decodeEnvelope(t, rr)
	errPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", payload["error"])
	}
	if errPayload["category"] != string(apperrors.CategoryAuth) {
		t.Fatalf("expected category %s, got %v", apperrors.CategoryAuth, errPayload["category"])
	}
}

func TestExtractIntegration_RateLimitIncludesMetadataAndHeader(t *testing.T) {
	ext := &scriptedExtractor{err: errors.New("HTTP 429: Too Many Requests")}
	svc := registerMockService(t, "ratelimitplatform", regexp.MustCompile(`^https://integration-ratelimit\.example/.*$`), ext, 1)
	h := newIntegrationHandler(t, svc)

	rr := postExtract(t, h, "https://integration-ratelimit.example/video/123")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}

	payload := decodeEnvelope(t, rr)
	errPayload, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", payload["error"])
	}
	if errPayload["category"] != string(apperrors.CategoryRateLimit) {
		t.Fatalf("expected category %s, got %v", apperrors.CategoryRateLimit, errPayload["category"])
	}

	metadata, ok := errPayload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %T", errPayload["metadata"])
	}
	if metadata["retryAfter"] == nil {
		t.Fatal("expected metadata.retryAfter")
	}
	if metadata["resetAt"] == nil {
		t.Fatal("expected metadata.resetAt")
	}
}

func TestExtractIntegration_CacheEffectivenessRepeatedRequest(t *testing.T) {
	ext := &scriptedExtractor{result: &core.ExtractResult{}}
	base := registerMockService(t, "cacheplatform", regexp.MustCompile(`^https://integration-cache\.example/.*$`), ext, 3)
	cached := extraction.NewCachedService(base, cache.NewPlatformTTLConfig(time.Minute, nil))
	h := newIntegrationHandler(t, cached)

	first := postExtract(t, h, "https://integration-cache.example/video/123")
	if first.Code != http.StatusOK {
		t.Fatalf("expected first status %d, got %d", http.StatusOK, first.Code)
	}

	second := postExtract(t, h, "https://integration-cache.example/video/123")
	if second.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, second.Code)
	}

	if ext.calls != 1 {
		t.Fatalf("expected cached second request to avoid extractor call, calls=%d", ext.calls)
	}
}
