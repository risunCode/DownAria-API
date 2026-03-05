package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"downaria-api/internal/shared/security"
)

var (
	defaultClient *http.Client
	clientOnce    sync.Once
)

const (
	defaultRequestTimeout        = 30 * time.Second
	defaultDialTimeout           = 10 * time.Second
	defaultKeepAliveTimeout      = 30 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 30 * time.Second
	defaultIdleConnTimeout       = 90 * time.Second
)

type HTTPClientOptions struct {
	RequestTimeout        time.Duration
	DialTimeout           time.Duration
	KeepAliveTimeout      time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	Validator             *security.OutboundURLValidator
}

func init() {
	GetDefaultClient()
}

func NewHTTPClient(timeoutSeconds int) *http.Client {
	timeout := defaultRequestTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return NewHTTPClientWithOptions(HTTPClientOptions{RequestTimeout: timeout})
}

func NewHTTPClientWithGuard(timeoutSeconds int, validator *security.OutboundURLValidator) *http.Client {
	timeout := defaultRequestTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return NewHTTPClientWithOptions(HTTPClientOptions{RequestTimeout: timeout, Validator: validator})
}

func NewHTTPClientWithOptions(opts HTTPClientOptions) *http.Client {
	normalized := normalizeHTTPClientOptions(opts)

	dialer := &net.Dialer{
		Timeout:   normalized.DialTimeout,
		KeepAlive: normalized.KeepAliveTimeout,
	}

	dialContext := dialer.DialContext
	if normalized.Validator != nil {
		dialContext = func(ctx context.Context, networkType string, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			host = strings.TrimSpace(host)
			if host == "" {
				return nil, fmt.Errorf("blocked host")
			}
			if err := normalized.Validator.ValidateHost(ctx, host); err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, networkType, addr)
		}
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       normalized.IdleConnTimeout,
		DisableKeepAlives:     false,
		DialContext:           dialContext,
		TLSHandshakeTimeout:   normalized.TLSHandshakeTimeout,
		ResponseHeaderTimeout: normalized.ResponseHeaderTimeout,
		ForceAttemptHTTP2:     true,
		ExpectContinueTimeout: 1 * time.Second,
	}

	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if normalized.Validator == nil || req == nil || req.URL == nil {
			return nil
		}
		_, err := normalized.Validator.Validate(req.Context(), req.URL.String())
		return err
	}

	return &http.Client{
		Timeout:       normalized.RequestTimeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
}

func normalizeHTTPClientOptions(opts HTTPClientOptions) HTTPClientOptions {
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = defaultDialTimeout
	}
	if opts.KeepAliveTimeout <= 0 {
		opts.KeepAliveTimeout = defaultKeepAliveTimeout
	}
	if opts.TLSHandshakeTimeout <= 0 {
		opts.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	}
	if opts.ResponseHeaderTimeout <= 0 {
		opts.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if opts.IdleConnTimeout <= 0 {
		opts.IdleConnTimeout = defaultIdleConnTimeout
	}
	return opts
}

func GetDefaultClient() *http.Client {
	clientOnce.Do(func() {
		defaultClient = NewHTTPClient(30)
	})
	return defaultClient
}

func GetClientWithTimeout(timeout time.Duration) *http.Client {
	return GetClientWithTimeoutGuard(timeout, nil)
}

func GetClientWithTimeoutGuard(timeout time.Duration, validator *security.OutboundURLValidator) *http.Client {
	return NewHTTPClientWithOptions(HTTPClientOptions{RequestTimeout: timeout, Validator: validator})
}
