package middleware

import (
	"context"
	"net/http"
	"time"

	"downaria-api/internal/shared/logger"
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

		// Log with appropriate severity based on status code
		logAttrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"latency_ms", latencyMS,
		}

		switch {
		case recorder.statusCode >= 500:
			logger.Error("HTTP request", logAttrs...)
		case recorder.statusCode >= 400:
			logger.Warn("HTTP request", logAttrs...)
		default:
			logger.Info("HTTP request", logAttrs...)
		}
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
