package errors

import "net/http"

const (
	CodeTimeout          = "TIMEOUT"
	CodeRateLimited429   = "RATE_LIMITED_429"
	CodeAuthRequired     = "AUTH_REQUIRED"
	CodePlatformNotFound = "PLATFORM_NOT_FOUND"
	CodeExtractionFailed = "EXTRACTION_FAILED"
	CodeNetworkError     = "NETWORK_ERROR"

	CodeInvalidJSON         = "INVALID_JSON"
	CodeInvalidURL          = "INVALID_URL"
	CodeUnsupportedPlatform = "UNSUPPORTED_PLATFORM"
	CodeInvalidSource       = "INVALID_SOURCE"
	CodeNoMediaFound        = "NO_MEDIA_FOUND"
	CodeUpstreamTimeout     = "UPSTREAM_TIMEOUT"
	CodeUpstreamRateLimited = "UPSTREAM_RATE_LIMITED"
	CodeUpstreamForbidden   = "UPSTREAM_FORBIDDEN"
	CodeUpstreamError       = "UPSTREAM_ERROR"
	CodeMethodNotAllowed    = "METHOD_NOT_ALLOWED"
	CodeNotFound            = "NOT_FOUND"
	CodeRateLimited         = "RATE_LIMITED"
	CodeOriginNotAllowed    = "ORIGIN_NOT_ALLOWED"
	CodeAccessDenied        = "ACCESS_DENIED"
	CodeMergeFailed         = "MERGE_FAILED"
	CodeFFmpegUnavailable   = "FFMPEG_UNAVAILABLE"
	CodeMissingParams       = "MISSING_PARAMS"
	CodeProxyFailed         = "PROXY_FAILED"
	CodeFileTooLarge        = "FILE_TOO_LARGE"
	CodeLoginRequired       = "LOGIN_REQUIRED"
)

func Message(code string) string {
	switch code {
	case CodeTimeout:
		return "request timeout"
	case CodeRateLimited429:
		return "too many requests"
	case CodeAuthRequired:
		return "authentication required"
	case CodePlatformNotFound:
		return "platform not supported"
	case CodeExtractionFailed:
		return "extraction failed"
	case CodeNetworkError:
		return "network error"
	case CodeInvalidJSON:
		return "request body must be valid JSON"
	case CodeInvalidURL:
		return "url is required and must be a valid http/https url"
	case CodeUnsupportedPlatform:
		return "unsupported platform"
	case CodeInvalidSource:
		return "invalid source url"
	case CodeNoMediaFound:
		return "no media found"
	case CodeUpstreamTimeout:
		return "upstream timeout"
	case CodeUpstreamRateLimited:
		return "upstream rate limited"
	case CodeUpstreamForbidden:
		return "upstream forbidden"
	case CodeUpstreamError:
		return "upstream error"
	case CodeMethodNotAllowed:
		return "method not allowed"
	case CodeNotFound:
		return "route not found"
	case CodeRateLimited:
		return "rate limit exceeded"
	case CodeOriginNotAllowed:
		return "request origin is not allowed"
	case CodeAccessDenied:
		return "access denied"
	case CodeMergeFailed:
		return "merge failed"
	case CodeFFmpegUnavailable:
		return "ffmpeg is not installed on server"
	case CodeMissingParams:
		return "required parameters are missing"
	case CodeProxyFailed:
		return "proxy failed"
	case CodeFileTooLarge:
		return "file exceeds maximum allowed size"
	case CodeLoginRequired:
		return "This content requires authentication. Please provide a cookie in Settings > Cookies to access private, age-restricted, or login-required content."
	default:
		return "request failed"
	}
}

func HTTPStatus(code string) int {
	switch code {
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeRateLimited429:
		return http.StatusTooManyRequests
	case CodeAuthRequired:
		return http.StatusUnauthorized
	case CodePlatformNotFound:
		return http.StatusNotFound
	case CodeNetworkError:
		return http.StatusBadGateway
	case CodeExtractionFailed:
		return http.StatusInternalServerError
	case CodeInvalidJSON, CodeInvalidURL, CodeInvalidSource, CodeMissingParams:
		return http.StatusBadRequest
	case CodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case CodeNotFound:
		return http.StatusNotFound
	case CodeOriginNotAllowed, CodeAccessDenied, CodeUpstreamForbidden:
		return http.StatusForbidden
	case CodeLoginRequired:
		return http.StatusUnauthorized
	case CodeRateLimited, CodeUpstreamRateLimited:
		return http.StatusTooManyRequests
	case CodeNoMediaFound:
		return http.StatusUnprocessableEntity
	case CodeUpstreamTimeout:
		return http.StatusGatewayTimeout
	case CodeFFmpegUnavailable:
		return http.StatusServiceUnavailable
	case CodeFileTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeProxyFailed, CodeUpstreamError:
		return http.StatusBadGateway
	case CodeMergeFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
