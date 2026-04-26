package extract

import (
	"errors"
	"strings"
)

const (
	ErrCodeNoMedia        = "no_media"
	ErrMsgInvalidURL      = "invalid url"
	ErrMsgAuthRequired    = "authentication is required"
	ErrMsgUpstreamFailure = "upstream request failed"
	ErrMsgNoMediaFound    = "no media found"
)

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *AppError) Is(target error) bool {
	other, ok := target.(*AppError)
	return ok && e.Kind != "" && e.Kind == other.Kind
}
func Wrap(kind Kind, message string, err error) *AppError {
	return &AppError{Kind: kind, Code: defaultCode(kind), Message: message, Retryable: defaultRetryable(kind), Err: err}
}

func WrapCode(kind Kind, code, message string, retryable bool, err error) *AppError {
	if strings.TrimSpace(code) == "" {
		code = defaultCode(kind)
	}
	return &AppError{Kind: kind, Code: code, Message: message, Retryable: retryable, Err: err}
}
func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return nil
}
func IsKind(err error, kind Kind) bool { return errors.Is(err, &AppError{Kind: kind}) }

func SafeMessage(err error) string {
	appErr := AsAppError(err)
	if appErr == nil {
		return "internal server error"
	}
	if appErr.Message != "" {
		return appErr.Message
	}
	switch appErr.Kind {
	case KindInvalidInput:
		return "invalid input"
	case KindUnsupportedPlatform:
		return "platform is not supported"
	case KindAuthRequired:
		return "authentication is required"
	case KindUpstreamFailure:
		return "upstream request failed"
	case KindExtractionFailed:
		return "extraction failed"
	case KindDownloadFailed:
		return "download failed"
	case KindMergeFailed:
		return "merge failed"
	case KindConvertFailed:
		return "conversion failed"
	case KindTimeout:
		return "operation timed out"
	default:
		return "internal server error"
	}
}

func (e *AppError) CodeValue() string {
	if e == nil {
		return string(KindInternal)
	}
	if strings.TrimSpace(e.Code) != "" {
		return e.Code
	}
	return defaultCode(e.Kind)
}

func defaultCode(kind Kind) string {
	switch kind {
	case KindInvalidInput:
		return "invalid_input"
	case KindUnsupportedPlatform:
		return "unsupported_platform"
	case KindAuthRequired:
		return "auth_required"
	case KindUpstreamFailure:
		return "upstream_failure"
	case KindExtractionFailed:
		return "extract_failed"
	case KindDownloadFailed:
		return "download_failed"
	case KindMergeFailed:
		return "merge_failed"
	case KindConvertFailed:
		return "convert_failed"
	case KindTimeout:
		return "timeout"
	default:
		return "internal"
	}
}

func defaultRetryable(kind Kind) bool {
	switch kind {
	case KindUpstreamFailure, KindDownloadFailed, KindTimeout:
		return true
	default:
		return false
	}
}
