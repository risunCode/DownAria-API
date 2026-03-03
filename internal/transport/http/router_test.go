package httptransport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"downaria-api/internal/core/config"
	"downaria-api/internal/transport/http/handlers"
)

func TestRouter_HealthEndpoint(t *testing.T) {
	cfg := config.Config{}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["success"] != true {
		t.Error("expected success to be true")
	}
}

func TestRouter_NotFound(t *testing.T) {
	cfg := config.Config{}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	cfg := config.Config{}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	// Try POST on GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestRouter_PublicAPI_Extract(t *testing.T) {
	cfg := config.Config{
		MaxDownloadSizeMB: 100,
	}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	body := map[string]string{
		"url": "not-a-valid-url",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/extract", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for routed extract handler, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestRouter_ProtectedAPI_RequiresOrigin(t *testing.T) {
	cfg := config.Config{
		AllowedOrigins: []string{"https://example.com"},
	}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	body := map[string]string{
		"url": "https://unknown-platform.com/video/123",
	}
	jsonBody, _ := json.Marshal(body)

	// Request without proper origin
	req := httptest.NewRequest(http.MethodPost, "/api/web/extract", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Should be blocked by origin middleware
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d for blocked origin, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRouter_PublicStats(t *testing.T) {
	cfg := config.Config{}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be an object")
	}

	// Verify stats schema
	requiredFields := []string{"todayVisits", "totalVisits", "totalExtractions", "totalDownloads"}
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			t.Errorf("missing required field: %s", field)
		}
	}
}

func TestRouter_Settings(t *testing.T) {
	cfg := config.Config{
		MaxDownloadSizeMB: 500,
	}
	h := handlers.NewHandler(cfg, time.Now())
	router := NewRouter(h, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
