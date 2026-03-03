package network

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	defaultClient *http.Client
	clientOnce    sync.Once
)

func init() {
	GetDefaultClient()
}

func NewHTTPClient(timeoutSeconds int) *http.Client {
	timeout := 30 * time.Second
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       20,
		IdleConnTimeout:       90 * time.Second,
		DisableKeepAlives:     false,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func GetDefaultClient() *http.Client {
	clientOnce.Do(func() {
		defaultClient = NewHTTPClient(30)
	})
	return defaultClient
}

func GetClientWithTimeout(timeout time.Duration) *http.Client {
	client := GetDefaultClient()
	return &http.Client{
		Timeout:   timeout,
		Transport: client.Transport,
	}
}
