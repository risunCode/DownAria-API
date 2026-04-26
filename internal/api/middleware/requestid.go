package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"
const requestStartedAtKey contextKey = "request_started_at"

type Middleware func(http.Handler) http.Handler

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 8)
			if _, err := rand.Read(buf); err != nil {
				buf = []byte("00000000")
			}
			requestID := hex.EncodeToString(buf)
			w.Header().Set("X-Request-ID", requestID)
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			ctx = context.WithValue(ctx, requestStartedAtKey, time.Now())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func StartedAtFromContext(ctx context.Context) time.Time {
	value, _ := ctx.Value(requestStartedAtKey).(time.Time)
	return value
}

func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}
