package middleware

import (
	"net/http"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/pkg/response"
)

func RequireMergeEnabled(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "merge endpoint is disabled")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
