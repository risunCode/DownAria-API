package errors

import (
	stdErrors "errors"
	"strings"
)

func MapExtractionError(err error) (int, string, string) {
	if err == nil {
		return HTTPStatus(CodeUpstreamError), CodeUpstreamError, Message(CodeUpstreamError)
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "invalid") || strings.Contains(msg, "unsupported") || strings.Contains(msg, "missing"):
		return HTTPStatus(CodeInvalidSource), CodeInvalidSource, err.Error()
	case strings.Contains(msg, "no media") || strings.Contains(msg, "not found"):
		return HTTPStatus(CodeNoMediaFound), CodeNoMediaFound, err.Error()
	case strings.Contains(msg, "timeout"):
		return HTTPStatus(CodeTimeout), CodeTimeout, err.Error()
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests"):
		return HTTPStatus(CodeRateLimited), CodeRateLimited, err.Error()
	case strings.Contains(msg, "403") || strings.Contains(msg, "forbidden") || strings.Contains(msg, "access denied"):
		return HTTPStatus(CodeAccessDenied), CodeAccessDenied, err.Error()
	case strings.Contains(msg, "login") || strings.Contains(msg, "authentication") || strings.Contains(msg, "auth"):
		return HTTPStatus(CodeAuthRequired), CodeAuthRequired, err.Error()
	default:
		var netErr interface{ Timeout() bool }
		if stdErrors.As(err, &netErr) && netErr.Timeout() {
			return HTTPStatus(CodeTimeout), CodeTimeout, err.Error()
		}
		return HTTPStatus(CodeExtractionFailed), CodeExtractionFailed, err.Error()
	}
}
