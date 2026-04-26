package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestRateLimiterRespectsBurstAndRefill(t *testing.T) {
	limiter := NewRateLimiter(2, 2)
	now := time.Now()
	limiter.now = func() time.Time { return now }

	if !limiter.Allow("client") {
		t.Fatal("first request should pass")
	}
	if !limiter.Allow("client") {
		t.Fatal("second burst request should pass")
	}
	if limiter.Allow("client") {
		t.Fatal("third burst request should be limited")
	}

	now = now.Add(500 * time.Millisecond)
	if !limiter.Allow("client") {
		t.Fatal("half-second refill at 2 rps should allow one request")
	}
	if limiter.Allow("client") {
		t.Fatal("refill should have consumed one token")
	}
}

func TestRateLimitUsesSameIPAcrossDifferentPorts(t *testing.T) {
	limiter := NewRateLimiter(1, 1)
	handler := RateLimit(limiter, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req1.RemoteAddr = "203.0.113.10:1111"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", rec1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req2.RemoteAddr = "203.0.113.10:2222"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request with same IP different port status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitSetsRetryAfterBasedOnLimiterRate(t *testing.T) {
	limiter := NewRateLimiter(20.0/60.0, 1)
	handler := RateLimit(limiter, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req1.RemoteAddr = "203.0.113.20:1111"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", rec1.Code, http.StatusOK)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	req2.RemoteAddr = "203.0.113.20:2222"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}

	retryAfter := rec2.Header().Get("Retry-After")
	seconds, err := strconv.Atoi(retryAfter)
	if err != nil {
		t.Fatalf("invalid Retry-After %q: %v", retryAfter, err)
	}
	if seconds < 2 {
		t.Fatalf("Retry-After too small: got %d, expected >= 2", seconds)
	}
}
