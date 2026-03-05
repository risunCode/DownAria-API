package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFeatureGateAllow(t *testing.T) {
	g := FeatureGate{Enabled: true, Rollout: 100}
	if !g.Allow("k") {
		t.Fatalf("expected allow")
	}
	if (FeatureGate{Enabled: false, Rollout: 100}).Allow("k") {
		t.Fatalf("expected deny")
	}
}

func TestRequireFeature(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true; w.WriteHeader(http.StatusOK) })
	h := RequireFeature(FeatureGate{Enabled: false, Rollout: 100}, nil)(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable || nextCalled {
		t.Fatalf("expected gated request")
	}
}
