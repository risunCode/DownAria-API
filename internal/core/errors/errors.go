package errors

import (
	"context"
	stdErrors "errors"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Sentinel errors for common validation cases
var (
	ErrInvalidURL             = stdErrors.New("invalid url")
	ErrUnsupportedPlatform    = stdErrors.New("unsupported platform")
	ErrMissingHost            = stdErrors.New("missing host")
	ErrUnsupportedScheme      = stdErrors.New("unsupported scheme")
	ErrHLSPlaylistParseFailed = stdErrors.New("hls playlist parse failed")
	ErrHLSSegmentFetchFailed  = stdErrors.New("hls segment fetch failed")
	ErrWorkerPoolFull         = stdErrors.New("worker pool queue is full")
)

type ErrorCategory string

const (
	CategoryNetwork          ErrorCategory = "NETWORK"
	CategoryValidation       ErrorCategory = "VALIDATION"
	CategoryRateLimit        ErrorCategory = "RATE_LIMIT"
	CategoryAuth             ErrorCategory = "AUTH"
	CategoryNotFound         ErrorCategory = "NOT_FOUND"
	CategoryExtractionFailed ErrorCategory = "EXTRACTION_FAILED"
)

type AppError struct {
	Category ErrorCategory  `json:"category"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
	err      error          `json:"-"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.err != nil {
		return e.err.Error()
	}
	return "request failed"
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *AppError) IsRetryable() bool {
	if e == nil {
		return false
	}
	switch e.Category {
	case CategoryNetwork, CategoryRateLimit:
		return true
	default:
		return false
	}
}

var httpStatusRegex = regexp.MustCompile(`(?i)\bhttp\s+(\d{3})\b`)

func CategorizeError(err error) *AppError {
	if err == nil {
		return &AppError{
			Category: CategoryExtractionFailed,
			Code:     CodeExtractionFailed,
			Message:  Message(CodeExtractionFailed),
		}
	}

	var existing *AppError
	if stdErrors.As(err, &existing) {
		return existing
	}

	if stdErrors.Is(err, context.DeadlineExceeded) {
		return &AppError{
			Category: CategoryNetwork,
			Code:     CodeTimeout,
			Message:  Message(CodeTimeout),
			err:      err,
		}
	}

	var netErr net.Error
	if stdErrors.As(err, &netErr) {
		if netErr.Timeout() {
			return &AppError{
				Category: CategoryNetwork,
				Code:     CodeTimeout,
				Message:  Message(CodeTimeout),
				err:      err,
			}
		}
		return &AppError{
			Category: CategoryNetwork,
			Code:     CodeNetworkError,
			Message:  Message(CodeNetworkError),
			err:      err,
		}
	}

	status := parseHTTPStatus(err.Error())
	if status != 0 {
		switch status {
		case 429:
			return &AppError{
				Category: CategoryRateLimit,
				Code:     CodeRateLimited,
				Message:  Message(CodeRateLimited),
				err:      err,
			}
		case 401, 403:
			code := CodeAuthRequired
			if status == 403 {
				code = CodeAccessDenied
			}
			return &AppError{
				Category: CategoryAuth,
				Code:     code,
				Message:  Message(code),
				Metadata: map[string]any{"requiresCookie": true},
				err:      err,
			}
		}
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))

	// Check sentinel errors first
	if stdErrors.Is(err, ErrUnsupportedScheme) || stdErrors.Is(err, ErrMissingHost) || stdErrors.Is(err, ErrInvalidURL) {
		return &AppError{
			Category: CategoryValidation,
			Code:     CodeInvalidURL,
			Message:  Message(CodeInvalidURL),
			err:      err,
		}
	}

	if stdErrors.Is(err, ErrUnsupportedPlatform) {
		return &AppError{
			Category: CategoryNotFound,
			Code:     CodePlatformNotFound,
			Message:  Message(CodePlatformNotFound),
			err:      err,
		}
	}

	if stdErrors.Is(err, ErrHLSPlaylistParseFailed) {
		return &AppError{Category: CategoryNetwork, Code: CodeHLSPlaylistParseFailed, Message: Message(CodeHLSPlaylistParseFailed), err: err}
	}
	if stdErrors.Is(err, ErrHLSSegmentFetchFailed) {
		return &AppError{Category: CategoryNetwork, Code: CodeHLSSegmentFetchFailed, Message: Message(CodeHLSSegmentFetchFailed), err: err}
	}
	if stdErrors.Is(err, ErrWorkerPoolFull) {
		return &AppError{Category: CategoryRateLimit, Code: CodeWorkerPoolFull, Message: Message(CodeWorkerPoolFull), err: err}
	}

	// Fallback to string matching for external errors
	switch {
	case strings.Contains(msg, "unsupported scheme") || strings.Contains(msg, "missing host") || strings.Contains(msg, "invalid url"):
		return &AppError{
			Category: CategoryValidation,
			Code:     CodeInvalidURL,
			Message:  Message(CodeInvalidURL),
			err:      err,
		}
	case strings.Contains(msg, "unsupported platform"):
		return &AppError{
			Category: CategoryNotFound,
			Code:     CodePlatformNotFound,
			Message:  Message(CodePlatformNotFound),
			err:      err,
		}
	default:
		return &AppError{
			Category: CategoryExtractionFailed,
			Code:     CodeExtractionFailed,
			Message:  err.Error(),
			err:      err,
		}
	}
}

func parseHTTPStatus(message string) int {
	if message == "" {
		return 0
	}
	match := httpStatusRegex.FindStringSubmatch(message)
	if len(match) < 2 {
		return 0
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return status
}
