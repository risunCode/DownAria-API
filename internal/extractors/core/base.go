package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fetchmoona/internal/infra/network"
)

// BaseExtractor provides common functionality for all extractors
type BaseExtractor struct {
	client *http.Client
}

// NewBaseExtractor creates a new base extractor with shared HTTP client
func NewBaseExtractor() *BaseExtractor {
	return &BaseExtractor{
		client: network.GetDefaultClient(),
	}
}

// MatchHost checks if the URL host matches any of the provided hosts
func (b *BaseExtractor) MatchHost(urlStr string, hosts []string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	for _, h := range hosts {
		if host == h || strings.Contains(host, h) {
			return true
		}
	}
	return false
}

// MakeRequest creates and executes an HTTP request with common headers
func (b *BaseExtractor) MakeRequest(method, url string, body io.Reader, opts ExtractOptions, extraHeaders map[string]string) (*http.Response, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Set User-Agent
	req.Header.Set("User-Agent", DefaultUserAgent)

	// Set additional headers
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	// Set extra platform-specific headers
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	// Set Cookie if provided
	if opts.Cookie != "" {
		req.Header.Set("Cookie", opts.Cookie)
	}

	// Use context timeout if specified
	client := b.client
	if opts.Timeout > 0 {
		client = network.GetClientWithTimeout(time.Duration(opts.Timeout) * time.Second)
	}

	return client.Do(req)
}

// CheckStatus validates HTTP response status
func (b *BaseExtractor) CheckStatus(resp *http.Response, expected int) error {
	if resp.StatusCode != expected {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}

// WrapError wraps errors with platform context
func (b *BaseExtractor) WrapError(platform, msg string, err error) error {
	if err != nil {
		return fmt.Errorf("%s extraction failed: %s (%v)", platform, msg, err)
	}
	return fmt.Errorf("%s extraction failed: %s", platform, msg)
}
