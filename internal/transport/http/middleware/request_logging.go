package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"downaria-api/internal/shared/util"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseRecorder) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = util.GenerateRequestID()
		}

		r.Header.Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		ctx = context.WithValue(ctx, "request_id", requestID)
		ctx = context.WithValue(ctx, "requestId", requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func StructuredLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		latencyMS := time.Since(start).Milliseconds()
		requestID := RequestIDFromContext(r.Context())
		log.Printf("request_id=%s method=%s path=%s status=%d latency_ms=%d", requestID, r.Method, r.URL.Path, recorder.statusCode, latencyMS)
	})
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, ok := ctx.Value(requestIDContextKey).(string)
	if !ok {
		return ""
	}
	return value
}
