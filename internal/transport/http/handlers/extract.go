package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fetchmoona/internal/app/services/extraction"
	apperrors "fetchmoona/internal/core/errors"
	"fetchmoona/internal/transport/http/middleware"
	"fetchmoona/pkg/response"
)

type extractRequest struct {
	URL    string `json:"url"`
	Cookie string `json:"cookie,omitempty"`
}

func (h *Handler) Extract(w http.ResponseWriter, r *http.Request) {
	builder := response.NewBuilderFromRequest(r).
		WithAccessMode("public").
		WithPublicContent(true)

	defer r.Body.Close()

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var req extractRequest
	if err := decoder.Decode(&req); err != nil {
		_ = builder.WriteErrorWithDetails(
			w,
			apperrors.HTTPStatus(apperrors.CodeInvalidJSON),
			apperrors.CodeInvalidJSON,
			apperrors.Message(apperrors.CodeInvalidJSON),
			string(apperrors.CategoryValidation),
			nil,
		)
		return
	}

	validatedURL, err := h.sanitizeAndValidateOutboundURL(r.Context(), req.URL)
	if err != nil {
		_ = builder.WriteErrorWithDetails(
			w,
			apperrors.HTTPStatus(apperrors.CodeInvalidURL),
			apperrors.CodeInvalidURL,
			apperrors.Message(apperrors.CodeInvalidURL),
			string(apperrors.CategoryValidation),
			nil,
		)
		return
	}
	req.URL = validatedURL

	builder.WithCookieSource(cookieSourceLabel(strings.TrimSpace(req.Cookie)))

	result, err := h.extractor.Extract(r.Context(), extraction.ExtractInput{
		URL:    req.URL,
		Cookie: req.Cookie,
	})
	if err != nil {
		status, code, message, category, metadata, retryAfter := h.mapExtractError(err)
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		}
		_ = builder.WriteErrorWithDetails(w, status, code, message, category, metadata)
		return
	}

	h.enrichVariantSizes(r.Context(), result, middleware.RequestIDFromContext(r.Context()))

	h.statsStore.RecordExtraction(time.Now().UTC())
	builder.WithCookieSource(cookieSourceLabel(string(result.Authentication.Source)))
	if result.Authentication.Used {
		builder.WithAccessMode("private").WithPublicContent(false)
	}

	_ = builder.WriteSuccess(w, result)
}

func cookieSourceLabel(source string) string {
	switch source {
	case "client":
		return "userProvided"
	case "server":
		return "server"
	default:
		return "guest"
	}
}

func (h *Handler) mapExtractError(err error) (status int, code, message, category string, metadata map[string]any, retryAfter int) {
	if errors.Is(err, extraction.ErrInvalidURL) {
		code = apperrors.CodeInvalidURL
		message = apperrors.Message(code)
		status = apperrors.HTTPStatus(code)
		category = string(apperrors.CategoryValidation)
		return
	}

	if errors.Is(err, extraction.ErrUnsupportedPlatform) {
		code = apperrors.CodePlatformNotFound
		message = apperrors.Message(code)
		status = apperrors.HTTPStatus(code)
		category = string(apperrors.CategoryNotFound)
		return
	}

	appErr := apperrors.CategorizeError(err)
	code = appErr.Code
	message = appErr.Message
	status = apperrors.HTTPStatus(code)
	category = string(appErr.Category)
	metadata = copyAnyMap(appErr.Metadata)

	if appErr.Category == apperrors.CategoryExtractionFailed {
		legacyStatus, legacyCode, legacyMessage := apperrors.MapExtractionError(err)
		status, code, message = legacyStatus, legacyCode, legacyMessage
		category = categoryFromCode(legacyCode)
	} else if category == "" {
		category = categoryFromCode(code)
	}

	canonicalCode, canonicalCause := canonicalizeErrorCode(code)
	if canonicalCode != code {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["causeCode"] = canonicalCause
		code = canonicalCode
		status = apperrors.HTTPStatus(code)
	}

	if message == "" {
		message = apperrors.Message(code)
	}

	if category == string(apperrors.CategoryRateLimit) {
		if metadata == nil {
			metadata = map[string]any{}
		}
		var resetAt int64
		retryAfter, resetAt = h.ensureRateLimitMetadata(metadata)
		if retryAfter > 0 {
			metadata["retryAfter"] = retryAfter
			metadata["resetAt"] = resetAt
		}
		if _, exists := metadata["limit"]; !exists {
			metadata["limit"] = h.config.GlobalRateLimitLimit
		}
		if _, exists := metadata["window"]; !exists {
			metadata["window"] = formatRateLimitWindow(h.config.GlobalRateLimitWindow)
		}
	}

	if status == 0 {
		status = http.StatusInternalServerError
	}

	return
}

func canonicalizeErrorCode(code string) (canonical string, cause string) {
	switch code {
	case apperrors.CodeRateLimited429, apperrors.CodeUpstreamRateLimited:
		return apperrors.CodeRateLimited, code
	case apperrors.CodeUpstreamTimeout:
		return apperrors.CodeTimeout, code
	case apperrors.CodeUpstreamForbidden, apperrors.CodeLoginRequired:
		return apperrors.CodeAuthRequired, code
	case apperrors.CodeUpstreamError:
		return apperrors.CodeExtractionFailed, code
	default:
		return code, code
	}
}

func categoryFromCode(code string) string {
	switch code {
	case apperrors.CodeInvalidJSON, apperrors.CodeInvalidURL, apperrors.CodeInvalidSource, apperrors.CodeMissingParams:
		return string(apperrors.CategoryValidation)
	case apperrors.CodePlatformNotFound, apperrors.CodeUnsupportedPlatform, apperrors.CodeNoMediaFound, apperrors.CodeNotFound:
		return string(apperrors.CategoryNotFound)
	case apperrors.CodeTimeout, apperrors.CodeUpstreamTimeout, apperrors.CodeNetworkError, apperrors.CodeProxyFailed, apperrors.CodeUpstreamError:
		return string(apperrors.CategoryNetwork)
	case apperrors.CodeRateLimited, apperrors.CodeRateLimited429, apperrors.CodeUpstreamRateLimited:
		return string(apperrors.CategoryRateLimit)
	case apperrors.CodeAuthRequired, apperrors.CodeLoginRequired, apperrors.CodeUpstreamForbidden, apperrors.CodeAccessDenied:
		return string(apperrors.CategoryAuth)
	default:
		return string(apperrors.CategoryExtractionFailed)
	}
}

func (h *Handler) ensureRateLimitMetadata(metadata map[string]any) (retryAfter int, resetAt int64) {
	if metadata == nil {
		metadata = map[string]any{}
	}

	retryAfter = int(h.config.GlobalRateLimitWindow.Seconds())
	if retryAfter < 1 {
		retryAfter = 60
	}
	if value, ok := metadata["retryAfter"]; ok {
		if parsed, ok := toInt(value); ok && parsed > 0 {
			retryAfter = parsed
		}
	}

	resetAt = time.Now().UTC().Add(time.Duration(retryAfter) * time.Second).Unix()
	if value, ok := metadata["resetAt"]; ok {
		if parsed, ok := toInt64(value); ok && parsed > 0 {
			resetAt = parsed
		}
	}

	return retryAfter, resetAt
}

func formatRateLimitWindow(window time.Duration) string {
	windowStr := strings.TrimSpace(window.String())
	if windowStr == "" {
		return "1m"
	}
	return windowStr
}

func toInt(value any) (int, bool) {
	if parsed, ok := toInt64(value); ok {
		return int(parsed), true
	}
	return 0, false
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > ^uint64(0)>>1 {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func copyAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
