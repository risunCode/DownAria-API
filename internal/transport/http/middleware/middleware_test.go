package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRequireOrigin(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("allowed origin passes", func(t *testing.T) {
		mw := RequireOrigin([]string{"https://example.com", "https://app.example.com"})
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("empty allowlist rejects all origins", func(t *testing.T) {
		mw := RequireOrigin(nil)
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://example.com")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "ORIGIN_NOT_ALLOWED") {
			t.Errorf("expected error code ORIGIN_NOT_ALLOWED in body, got %s", body)
		}
	})

	t.Run("blocked origin returns 403", func(t *testing.T) {
		mw := RequireOrigin([]string{"https://example.com"})
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://evil.com")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "ORIGIN_NOT_ALLOWED") {
			t.Errorf("expected error code ORIGIN_NOT_ALLOWED in body, got %s", body)
		}
	})

	t.Run("wildcard allows all", func(t *testing.T) {
		mw := RequireOrigin([]string{"*"})
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://any-origin.com")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("empty origin header returns 403", func(t *testing.T) {
		mw := RequireOrigin([]string{"https://example.com"})
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "ORIGIN_NOT_ALLOWED") {
			t.Errorf("expected error code ORIGIN_NOT_ALLOWED in body, got %s", body)
		}
	})
}

func TestBlockBotAccess(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mw := BlockBotAccess()
	handler := mw(nextHandler)

	t.Run("browser User-Agent passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
	})

	t.Run("curl User-Agent is blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "curl/7.68.0")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "ACCESS_DENIED") {
			t.Errorf("expected error code ACCESS_DENIED in body, got %s", body)
		}
	})

	t.Run("empty User-Agent is blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
		}

		body := rr.Body.String()
		if !strings.Contains(body, "ACCESS_DENIED") {
			t.Errorf("expected error code ACCESS_DENIED in body, got %s", body)
		}
	})

	t.Run("bot User-Agent is blocked", func(t *testing.T) {
		botUserAgents := []string{
			"Wget/1.20.3",
			"python-requests/2.25.1",
			"PostmanRuntime/7.26.8",
			"insomnia/2020.5.2",
			"Go-http-client/1.1",
			"node-fetch/1.0",
			"axios/0.21.1",
		}

		for _, ua := range botUserAgents {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("User-Agent", ua)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected status %d for User-Agent %q, got %d", http.StatusForbidden, ua, rr.Code)
			}
		}
	})
}

func TestRateLimit(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("requests within limit pass", func(t *testing.T) {
		limiter := NewRateLimiter(5, time.Minute)
		mw := RateLimit(limiter)
		handler := mw(nextHandler)

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, rr.Code)
			}
		}
	})

	t.Run("request beyond limit is blocked", func(t *testing.T) {
		limiter := NewRateLimiter(2, time.Minute)
		mw := RateLimit(limiter)
		handler := mw(nextHandler)

		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.2:12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, rr.Code)
		}

		retryAfter := rr.Header().Get("Retry-After")
		if retryAfter == "" {
			t.Errorf("expected Retry-After header to be set")
		}

		body := rr.Body.String()
		if !strings.Contains(body, "RATE_LIMITED") {
			t.Errorf("expected error code RATE_LIMITED in body, got %s", body)
		}
		if !strings.Contains(body, "retryAfter") {
			t.Errorf("expected retryAfter metadata in body, got %s", body)
		}
	})

	t.Run("rate limit headers are set", func(t *testing.T) {
		limiter := NewRateLimiter(10, time.Minute)
		mw := RateLimit(limiter)
		handler := mw(nextHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.3:12345"
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		limit := rr.Header().Get("X-RateLimit-Limit")
		if limit != "10" {
			t.Errorf("expected X-RateLimit-Limit header to be 10, got %s", limit)
		}

		remaining := rr.Header().Get("X-RateLimit-Remaining")
		if remaining != "9" {
			t.Errorf("expected X-RateLimit-Remaining header to be 9, got %s", remaining)
		}

		reset := rr.Header().Get("X-RateLimit-Reset")
		if reset == "" {
			t.Error("expected X-RateLimit-Reset header to be set")
		}
	})
}

func TestRateLimiter_EvictsWhenCapacityReached(t *testing.T) {
	r := NewRateLimiter(5, time.Minute)
	r.ConfigureBuckets(2, time.Hour)

	allowed, _, _ := r.Allow("1.1.1.1")
	if !allowed {
		t.Fatal("expected first ip allowed")
	}
	allowed, _, _ = r.Allow("2.2.2.2")
	if !allowed {
		t.Fatal("expected second ip allowed")
	}
	allowed, _, _ = r.Allow("3.3.3.3")
	if !allowed {
		t.Fatal("expected third ip allowed")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buckets) != 2 {
		t.Fatalf("expected bucket cap=2, got %d", len(r.buckets))
	}
}

func TestRateLimiter_EvictsStaleBuckets(t *testing.T) {
	r := NewRateLimiter(5, time.Minute)
	r.ConfigureBuckets(10, time.Millisecond)

	_, _, _ = r.Allow("1.1.1.1")
	time.Sleep(15 * time.Millisecond)
	_, _, _ = r.Allow("2.2.2.2")

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.buckets["1.1.1.1"]; ok {
		t.Fatalf("expected stale bucket to be evicted")
	}
}

func TestRequestID(t *testing.T) {
	requestIDHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := middleware.GetReqID(r.Context())
		if requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("request ID is generated if not present", func(t *testing.T) {
		mw := middleware.RequestID
		handler := mw(requestIDHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		requestID := rr.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("expected X-Request-ID header to be generated")
		}
	})

	t.Run("existing request ID is preserved", func(t *testing.T) {
		mw := middleware.RequestID
		handler := mw(requestIDHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "existing-request-id-123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		requestID := rr.Header().Get("X-Request-ID")
		if requestID != "existing-request-id-123" {
			t.Errorf("expected X-Request-ID header to be preserved as 'existing-request-id-123', got %s", requestID)
		}
	})

	t.Run("request ID is added to response headers", func(t *testing.T) {
		mw := middleware.RequestID
		handler := mw(requestIDHandler)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}

		requestID := rr.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("expected X-Request-ID header to be present in response")
		}
	})
}

func TestRouteRateLimit_AppliesOnlyConfiguredRoute(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	routeLimiter := NewRateLimiter(1, time.Minute)
	mw := RouteRateLimit([]RouteLimitRule{{Method: http.MethodPost, Path: "/api/v1/merge", Limiter: routeLimiter}})
	h := mw(nextHandler)

	postReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/merge", nil)
	postReq1.RemoteAddr = "192.168.1.90:12345"
	postRes1 := httptest.NewRecorder()
	h.ServeHTTP(postRes1, postReq1)
	if postRes1.Code != http.StatusOK {
		t.Fatalf("expected first merge request status %d, got %d", http.StatusOK, postRes1.Code)
	}

	postReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/merge", nil)
	postReq2.RemoteAddr = "192.168.1.90:12345"
	postRes2 := httptest.NewRecorder()
	h.ServeHTTP(postRes2, postReq2)
	if postRes2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second merge request status %d, got %d", http.StatusTooManyRequests, postRes2.Code)
	}

	cheapReq := httptest.NewRequest(http.MethodGet, "/api/v1/proxy", nil)
	cheapReq.RemoteAddr = "192.168.1.90:12345"
	cheapRes := httptest.NewRecorder()
	h.ServeHTTP(cheapRes, cheapReq)
	if cheapRes.Code != http.StatusOK {
		t.Fatalf("expected cheap route status %d, got %d", http.StatusOK, cheapRes.Code)
	}
}
