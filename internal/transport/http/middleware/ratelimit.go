package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "fetchmoona/internal/core/errors"
	"fetchmoona/internal/shared/util"
	"fetchmoona/pkg/response"
)

type ipBucket struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

type RateLimiter struct {
	limit          int
	window         time.Duration
	maxBuckets     int
	staleAfter     time.Duration
	mu             sync.Mutex
	buckets        map[string]*ipBucket
	clientIPLookup func(*http.Request) string
}

type RouteLimitRule struct {
	Method  string
	Path    string
	Limiter *RateLimiter
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit < 1 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	maxBuckets := 10000
	staleAfter := 10 * window
	if staleAfter < 5*time.Minute {
		staleAfter = 5 * time.Minute
	}
	return &RateLimiter{
		limit:          limit,
		window:         window,
		maxBuckets:     maxBuckets,
		staleAfter:     staleAfter,
		buckets:        make(map[string]*ipBucket),
		clientIPLookup: util.ClientIPFromRequest,
	}
}

func (rl *RateLimiter) ConfigureBuckets(maxBuckets int, staleAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if maxBuckets > 0 {
		rl.maxBuckets = maxBuckets
	}
	if staleAfter > 0 {
		rl.staleAfter = staleAfter
	}
}

func (rl *RateLimiter) SetClientIPLookup(fn func(*http.Request) string) {
	if fn == nil {
		return
	}
	rl.mu.Lock()
	rl.clientIPLookup = fn
	rl.mu.Unlock()
}

func (rl *RateLimiter) clientIP(r *http.Request) string {
	rl.mu.Lock()
	lookup := rl.clientIPLookup
	rl.mu.Unlock()
	if lookup == nil {
		return ""
	}
	return strings.TrimSpace(lookup(r))
}

func (rl *RateLimiter) Allow(ip string) (bool, int, int64) {
	now := time.Now().UTC()
	rounded := now.Truncate(rl.window)
	resetAt := rounded.Add(rl.window).Unix()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.evictStaleLocked(now)

	bucket, exists := rl.buckets[ip]
	if !exists || now.Sub(bucket.windowStart) >= rl.window {
		rl.ensureCapacityLocked(now, ip)
		rl.buckets[ip] = &ipBucket{windowStart: rounded, count: 1, lastSeen: now}
		return true, rl.limit - 1, resetAt
	}

	bucket.lastSeen = now
	if bucket.count >= rl.limit {
		return false, 0, resetAt
	}

	bucket.count++
	remaining := rl.limit - bucket.count
	return true, remaining, resetAt
}

func (rl *RateLimiter) evictStaleLocked(now time.Time) {
	if rl.staleAfter <= 0 {
		return
	}
	for ip, bucket := range rl.buckets {
		if bucket == nil {
			delete(rl.buckets, ip)
			continue
		}
		if now.Sub(bucket.lastSeen) > rl.staleAfter {
			delete(rl.buckets, ip)
		}
	}
}

func (rl *RateLimiter) ensureCapacityLocked(now time.Time, incomingIP string) {
	if rl.maxBuckets < 1 {
		return
	}
	if _, exists := rl.buckets[incomingIP]; exists {
		return
	}
	if len(rl.buckets) < rl.maxBuckets {
		return
	}

	rl.evictStaleLocked(now)
	if len(rl.buckets) < rl.maxBuckets {
		return
	}

	victimIP := ""
	var victimBucket *ipBucket
	for ip, bucket := range rl.buckets {
		if bucket == nil {
			victimIP = ip
			break
		}
		if victimBucket == nil || bucket.lastSeen.Before(victimBucket.lastSeen) || (bucket.lastSeen.Equal(victimBucket.lastSeen) && ip < victimIP) {
			victimIP = ip
			victimBucket = bucket
		}
	}
	if victimIP != "" {
		delete(rl.buckets, victimIP)
	}
}

func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !applyRateLimit(w, r, limiter) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RouteRateLimit(rules []RouteLimitRule) func(http.Handler) http.Handler {
	active := make([]RouteLimitRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Limiter == nil {
			continue
		}
		rule.Method = strings.ToUpper(strings.TrimSpace(rule.Method))
		rule.Path = strings.TrimSpace(rule.Path)
		if rule.Method == "" || rule.Path == "" {
			continue
		}
		active = append(active, rule)
	}

	return func(next http.Handler) http.Handler {
		if len(active) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, rule := range active {
				if r.Method == rule.Method && r.URL.Path == rule.Path {
					if !applyRateLimit(w, r, rule.Limiter) {
						return
					}
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func applyRateLimit(w http.ResponseWriter, r *http.Request, limiter *RateLimiter) bool {
	if limiter == nil {
		return true
	}

	ip := limiter.clientIP(r)
	if ip == "" {
		ip = "unknown"
	}
	allowed, remaining, resetAt := limiter.Allow(ip)

	w.Header().Set("X-RateLimit-Limit", intToString(limiter.limit))
	w.Header().Set("X-RateLimit-Remaining", intToString(remaining))
	w.Header().Set("X-RateLimit-Reset", int64ToString(resetAt))

	if allowed {
		return true
	}

	retryAfter := int(resetAt - time.Now().UTC().Unix())
	if retryAfter < 1 {
		retryAfter = 1
	}

	w.Header().Set("Retry-After", intToString(retryAfter))
	response.WriteErrorRequestWithDetails(
		w,
		r,
		apperrors.HTTPStatus(apperrors.CodeRateLimited),
		apperrors.CodeRateLimited,
		apperrors.Message(apperrors.CodeRateLimited),
		string(apperrors.CategoryRateLimit),
		map[string]any{
			"retryAfter": retryAfter,
			"resetAt":    resetAt,
			"limit":      limiter.limit,
			"window":     formatRateWindow(limiter.window),
			"status":     http.StatusTooManyRequests,
		},
	)
	return false
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func formatRateWindow(window time.Duration) string {
	windowStr := strings.TrimSpace(window.String())
	if windowStr == "" {
		return "1m"
	}
	return windowStr
}
