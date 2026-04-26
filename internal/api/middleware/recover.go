package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", slog.String("request_id", FromContext(r.Context())), slog.Any("panic", recovered))
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"kind": "internal", "code": "panic_recovered", "message": "internal server error", "request_id": FromContext(r.Context())}})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
