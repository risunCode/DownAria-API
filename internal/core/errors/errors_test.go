package errors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

func TestCategorizeError_Timeout_Context(t *testing.T) {
	err := context.DeadlineExceeded
	categorized := CategorizeError(err)
	if categorized.Category != CategoryNetwork {
		t.Fatalf("expected category %q, got %q", CategoryNetwork, categorized.Category)
	}
	if categorized.Code != CodeTimeout {
		t.Fatalf("expected code %q, got %q", CodeTimeout, categorized.Code)
	}
	if !categorized.IsRetryable() {
		t.Fatalf("expected retryable")
	}
}

func TestCategorizeError_Timeout_NetError(t *testing.T) {
	var err error = timeoutNetErr{}
	if _, ok := err.(net.Error); !ok {
		t.Fatalf("expected net.Error")
	}
	categorized := CategorizeError(err)
	if categorized.Category != CategoryNetwork {
		t.Fatalf("expected category %q, got %q", CategoryNetwork, categorized.Category)
	}
	if categorized.Code != CodeTimeout {
		t.Fatalf("expected code %q, got %q", CodeTimeout, categorized.Code)
	}
}

func TestCategorizeError_RateLimit429(t *testing.T) {
	categorized := CategorizeError(fmt.Errorf("HTTP 429: Too Many Requests"))
	if categorized.Category != CategoryRateLimit {
		t.Fatalf("expected category %q, got %q", CategoryRateLimit, categorized.Category)
	}
	if categorized.Code != CodeRateLimited {
		t.Fatalf("expected code %q, got %q", CodeRateLimited, categorized.Code)
	}
	if !categorized.IsRetryable() {
		t.Fatalf("expected retryable")
	}
}

func TestCategorizeError_Auth401(t *testing.T) {
	categorized := CategorizeError(fmt.Errorf("HTTP 401: Unauthorized"))
	if categorized.Category != CategoryAuth {
		t.Fatalf("expected category %q, got %q", CategoryAuth, categorized.Category)
	}
	if categorized.Code != CodeAuthRequired {
		t.Fatalf("expected code %q, got %q", CodeAuthRequired, categorized.Code)
	}
	if categorized.IsRetryable() {
		t.Fatalf("expected not retryable")
	}
	if categorized.Metadata == nil || categorized.Metadata["requiresCookie"] != true {
		t.Fatalf("expected requiresCookie metadata")
	}
}

func TestCategorizeError_Auth403(t *testing.T) {
	categorized := CategorizeError(fmt.Errorf("HTTP 403: Forbidden"))
	if categorized.Category != CategoryAuth {
		t.Fatalf("expected category %q, got %q", CategoryAuth, categorized.Category)
	}
	if categorized.Code != CodeAccessDenied {
		t.Fatalf("expected code %q, got %q", CodeAccessDenied, categorized.Code)
	}
}

func TestCategorizeError_InvalidURL(t *testing.T) {
	categorized := CategorizeError(errors.New("unsupported scheme"))
	if categorized.Category != CategoryValidation {
		t.Fatalf("expected category %q, got %q", CategoryValidation, categorized.Category)
	}
	if categorized.Code != CodeInvalidURL {
		t.Fatalf("expected code %q, got %q", CodeInvalidURL, categorized.Code)
	}
}

func TestCategorizeError_UnsupportedPlatform(t *testing.T) {
	categorized := CategorizeError(errors.New("unsupported platform for URL: https://example.com"))
	if categorized.Category != CategoryNotFound {
		t.Fatalf("expected category %q, got %q", CategoryNotFound, categorized.Category)
	}
	if categorized.Code != CodePlatformNotFound {
		t.Fatalf("expected code %q, got %q", CodePlatformNotFound, categorized.Code)
	}
}

func TestCategorizeError_Unknown(t *testing.T) {
	categorized := CategorizeError(errors.New("boom"))
	if categorized.Category != CategoryExtractionFailed {
		t.Fatalf("expected category %q, got %q", CategoryExtractionFailed, categorized.Category)
	}
	if categorized.Code != CodeExtractionFailed {
		t.Fatalf("expected code %q, got %q", CodeExtractionFailed, categorized.Code)
	}
	if categorized.Message != "boom" {
		t.Fatalf("expected message %q, got %q", "boom", categorized.Message)
	}
}

func TestCategorizeError_NetworkNonTimeout(t *testing.T) {
	categorized := CategorizeError(&net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true})
	if categorized.Category != CategoryNetwork {
		t.Fatalf("expected category %q, got %q", CategoryNetwork, categorized.Category)
	}
	if categorized.Code != CodeNetworkError {
		t.Fatalf("expected code %q, got %q", CodeNetworkError, categorized.Code)
	}
}

func TestCategorizeError_AppErrorPassthrough(t *testing.T) {
	in := &AppError{Category: CategoryAuth, Code: CodeAuthRequired, Message: "x"}
	categorized := CategorizeError(in)
	if categorized != in {
		t.Fatalf("expected same instance")
	}
}

func TestParseHTTPStatus(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"HTTP 429", 429},
		{"http429", 0},
		{"HTTP 401: Unauthorized", 401},
		{"something else", 0},
	}

	for _, tc := range cases {
		if got := parseHTTPStatus(tc.in); got != tc.want {
			t.Fatalf("parseHTTPStatus(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCategorizeError_ContextTimeoutWrapped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	categorized := CategorizeError(ctx.Err())
	if categorized.Category != CategoryNetwork {
		t.Fatalf("expected category %q, got %q", CategoryNetwork, categorized.Category)
	}
	if categorized.Code != CodeTimeout {
		t.Fatalf("expected code %q, got %q", CodeTimeout, categorized.Code)
	}
}
