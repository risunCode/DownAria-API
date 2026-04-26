package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*rateBucket
	now     func() time.Time
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

func NewRateLimiter(rate float64, burst int) *RateLimiter {
	if rate <= 0 || burst <= 0 {
		return nil
	}
	return &RateLimiter{
		rate:    rate,
		burst:   float64(burst),
		buckets: make(map[string]*rateBucket),
		now:     time.Now,
	}
}

func (l *RateLimiter) Allow(key string) bool {
	allowed, _ := l.AllowWithRetry(key)
	return allowed
}

func (l *RateLimiter) AllowWithRetry(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	if key == "" {
		key = "anonymous"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	bucket := l.buckets[key]
	if bucket == nil {
		l.buckets[key] = &rateBucket{tokens: l.burst - 1, last: now}
		return true, 0
	}

	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * l.rate
		if bucket.tokens > l.burst {
			bucket.tokens = l.burst
		}
		bucket.last = now
	}
	if bucket.tokens < 1 {
		if l.rate <= 0 {
			return false, time.Second
		}
		needed := 1 - bucket.tokens
		waitSeconds := needed / l.rate
		wait := time.Duration(math.Ceil(waitSeconds * float64(time.Second)))
		if wait < time.Second {
			wait = time.Second
		}
		return false, wait
	}
	bucket.tokens--
	return true, 0
}

func RateLimit(limiter *RateLimiter, keyFn func(*http.Request) string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			key := ""
			if keyFn != nil {
				key = keyFn(r)
			}
			if key == "" {
				key = clientKey(r)
			}
			if allowed, wait := limiter.AllowWithRetry(key); !allowed {
				seconds := int(math.Ceil(wait.Seconds()))
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return "ip:" + host
	}
	return "ip:" + r.RemoteAddr
}
