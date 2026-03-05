package network

import (
	"net"
	"net/http"
	"time"
)

type OptimizedClientOptions struct {
	Timeout         time.Duration
	MaxIdleConns    int
	MaxIdlePerHost  int
	MaxConnsPerHost int
}

func NewOptimizedHTTPClient(opts OptimizedClientOptions) *http.Client {
	if opts.MaxIdleConns <= 0 {
		opts.MaxIdleConns = 100
	}
	if opts.MaxIdlePerHost <= 0 {
		opts.MaxIdlePerHost = 10
	}
	if opts.MaxConnsPerHost <= 0 {
		opts.MaxConnsPerHost = 20
	}

	transport := &http.Transport{
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdlePerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	client := &http.Client{Transport: transport}
	if opts.Timeout > 0 {
		client.Timeout = opts.Timeout
	}

	return client
}
