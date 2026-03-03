package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuilderErrorWithDetails_IncludesCategoryAndMetadata(t *testing.T) {
	metadata := map[string]any{"retryAfter": 30, "resetAt": int64(1710000000)}
	b := NewBuilder().ErrorWithDetails("RATE_LIMITED", "too many requests", "RATE_LIMIT", metadata)

	metadata["retryAfter"] = 999

	resp := b.Build()
	if resp.Error == nil {
		t.Fatalf("expected error payload")
	}
	if resp.Error.Category != "RATE_LIMIT" {
		t.Fatalf("expected category RATE_LIMIT, got %q", resp.Error.Category)
	}
	if resp.Error.Metadata["retryAfter"] != 30 {
		t.Fatalf("expected retryAfter 30, got %v", resp.Error.Metadata["retryAfter"])
	}
}

func TestWriteErrorWithDetails_ResponseBodyContainsDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteErrorWithDetails(rr, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests", "RATE_LIMIT", map[string]any{"retryAfter": 60})

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, rr.Code)
	}

	var payload Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Error == nil {
		t.Fatalf("expected error payload")
	}
	if payload.Error.Category != "RATE_LIMIT" {
		t.Fatalf("expected category RATE_LIMIT, got %q", payload.Error.Category)
	}
	if payload.Error.Metadata["retryAfter"] != float64(60) {
		t.Fatalf("expected retryAfter 60, got %v", payload.Error.Metadata["retryAfter"])
	}
}

func TestWriteError_LegacyContractStillWorks(t *testing.T) {
	rr := httptest.NewRecorder()

	WriteError(rr, http.StatusBadRequest, "INVALID", "bad request")

	var payload Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Error == nil {
		t.Fatalf("expected error payload")
	}
	if payload.Error.Code != "INVALID" {
		t.Fatalf("expected code INVALID, got %q", payload.Error.Code)
	}
	if payload.Error.Category != "" {
		t.Fatalf("expected empty category, got %q", payload.Error.Category)
	}
}
