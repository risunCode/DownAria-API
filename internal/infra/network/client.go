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

func init() {
	GetDefaultClient()
}

func NewHTTPClient(timeoutSeconds int) *http.Client {
	return newHTTPClientWithGuard(timeoutSeconds, nil)
}

func NewHTTPClientWithGuard(timeoutSeconds int, validator *security.OutboundURLValidator) *http.Client {
	return newHTTPClientWithGuard(timeoutSeconds, validator)
}

func newHTTPClientWithGuard(timeoutSeconds int, validator *security.OutboundURLValidator) *http.Client {
	timeout := 30 * time.Second
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	dialContext := dialer.DialContext
	if validator != nil {
		dialContext = func(ctx context.Context, networkType string, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}
			host = strings.TrimSpace(host)
			if host == "" {
				return nil, fmt.Errorf("blocked host")
			}
			if err := validator.ValidateHost(ctx, host); err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, networkType, addr)
		}
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		DialContext:           dialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if validator == nil || req == nil || req.URL == nil {
			return nil
		}
		_, err := validator.Validate(req.Context(), req.URL.String())
		return err
	}

	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
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
	if validator != nil {
		seconds := int(timeout / time.Second)
		if timeout > 0 && seconds < 1 {
			seconds = 1
		}
		client := NewHTTPClientWithGuard(seconds, validator)
		client.Timeout = timeout
		return client
	}

	client := GetDefaultClient()
	return &http.Client{
		Timeout:       timeout,
		Transport:     client.Transport,
		CheckRedirect: client.CheckRedirect,
	}
}
