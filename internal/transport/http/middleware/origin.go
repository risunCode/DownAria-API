package middleware

import (
	"net/http"
	"strings"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/pkg/response"
)

func RequireOrigin(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			allowAll = true
		}
		allowed[trimmed] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowAll || len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeOriginNotAllowed), apperrors.CodeOriginNotAllowed, "origin header is required on /api/web routes")
				return
			}

			if _, ok := allowed[origin]; !ok {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeOriginNotAllowed), apperrors.CodeOriginNotAllowed, apperrors.Message(apperrors.CodeOriginNotAllowed))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
