package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUpstreamAuthorization_UsesHeaderValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy?upstream_auth=Bearer+query-token", nil)
	req.Header.Set("X-Upstream-Authorization", "Bearer header-token")

	got := resolveUpstreamAuthorization(req)

	if got != "Bearer header-token" {
		t.Fatalf("expected header authorization to be used, got %q", got)
	}
}

func TestResolveUpstreamAuthorization_IgnoresQueryParameter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proxy?upstream_auth=Bearer+query-token", nil)

	got := resolveUpstreamAuthorization(req)

	if got != "" {
		t.Fatalf("expected query parameter upstream_auth to be ignored, got %q", got)
	}
}
