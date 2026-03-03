package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Stack holds all middleware configuration and builds the chain
type Stack struct {
	RateLimiter   *RateLimiter
	AllowedOrigins []string
}

// NewStack creates a new middleware stack with the given configuration
func NewStack(allowedOrigins []string, rateLimit int, rateWindow time.Duration) *Stack {
	return &Stack{
		AllowedOrigins: allowedOrigins,
		RateLimiter:    NewRateLimiter(rateLimit, rateWindow),
	}
}

// DefaultStack creates a middleware stack with sensible defaults
func DefaultStack(allowedOrigins []string) *Stack {
	return NewStack(allowedOrigins, 60, time.Minute)
}

// Global returns middleware that should be applied to all routes
func (s *Stack) Global() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.RealIP,
		middleware.Logger,
		middleware.Recoverer,
		middleware.Timeout(60 * time.Second),
	}
}

// Protected returns middleware for protected routes (web API)
func (s *Stack) Protected() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		RequireOrigin(s.AllowedOrigins),
		BlockBotAccess(),
		RateLimit(s.RateLimiter),
	}
}

// Public returns middleware for public routes
func (s *Stack) Public() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		RateLimit(s.RateLimiter),
	}
}

// ApplyGlobal applies global middleware to the router
func (s *Stack) ApplyGlobal(r chi.Router) {
	for _, mw := range s.Global() {
		r.Use(mw)
	}
}

// ApplyProtected applies protected middleware to a route group
func (s *Stack) ApplyProtected(r chi.Router) {
	for _, mw := range s.Protected() {
		r.Use(mw)
	}
}

// ApplyPublic applies public middleware to a route group
func (s *Stack) ApplyPublic(r chi.Router) {
	for _, mw := range s.Public() {
		r.Use(mw)
	}
}
