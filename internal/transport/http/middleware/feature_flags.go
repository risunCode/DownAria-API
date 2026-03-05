package middleware

import (
	"crypto/sha1"
	"encoding/binary"
	"net/http"
	"strconv"
	"strings"
)

type FeatureGate struct {
	Enabled bool
	Rollout int
}

func (g FeatureGate) Allow(key string) bool {
	if !g.Enabled {
		return false
	}
	if g.Rollout >= 100 || key == "" {
		return true
	}
	if g.Rollout <= 0 {
		return false
	}
	h := sha1.Sum([]byte(key))
	v := binary.BigEndian.Uint32(h[:4]) % 100
	return int(v) < g.Rollout
}

func RequireFeature(g FeatureGate, fallback http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(RequestIDFromContext(r.Context()))
			if key == "" {
				key = strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
			}
			if !g.Allow(key) {
				if fallback != nil {
					fallback(w, r)
					return
				}
				w.Header().Set("Retry-After", strconv.Itoa(30))
				http.Error(w, "feature disabled", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
