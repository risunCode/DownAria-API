package middleware

import (
	"net/http"
	"strings"

	apperrors "fetchmoona/internal/core/errors"
	"fetchmoona/pkg/response"
)

func BlockBotAccess() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ua := strings.ToLower(r.Header.Get("User-Agent"))

			blockedPrefixes := []string{
				"curl",
				"wget",
				"postman",
				"insomnia",
				"python-requests",
				"go-http-client",
				"node-fetch",
				"axios",
				"libwww-perl",
			}

			if ua == "" {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "direct api access without browser is not allowed on web routes")
				return
			}

			for _, prefix := range blockedPrefixes {
				if strings.Contains(ua, prefix) {
					response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "automated tools or bot access is not allowed on web routes, use /api/v1 instead")
					return
				}
			}

			secFetchDest := r.Header.Get("Sec-Fetch-Dest")
			secFetchMode := r.Header.Get("Sec-Fetch-Mode")

			if secFetchDest == "" && secFetchMode == "" && !strings.Contains(ua, "mozilla") && !strings.Contains(ua, "chrome") && !strings.Contains(ua, "safari") {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "invalid browser signature")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
