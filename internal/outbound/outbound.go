package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type ValidationError struct{ Reason string }

func (e *ValidationError) Error() string {
	if e == nil || strings.TrimSpace(e.Reason) == "" {
		return "url validation failed"
	}
	return e.Reason
}

type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}
type Options struct {
	BlockedCIDRs []string
}
type Guard struct {
	resolver      Resolver
	blockedRanges []netip.Prefix
}
type HTTPClientOptions struct {
	Timeout         time.Duration
	Client          *http.Client
	MaxIdleConns    int
	MaxConnsPerHost int
	IdleConnTimeout time.Duration
}
type Client struct {
	httpClient      *http.Client
	guard           *Guard
	noTimeoutOnce   sync.Once
	noTimeoutClient *http.Client
}

func (c *Client) HTTPClient() *http.Client {
	if c == nil || c.httpClient == nil {
		return http.DefaultClient
	}
	return c.httpClient
}

func (c *Client) HTTPClientWithoutTimeout() *http.Client {
	if c == nil || c.httpClient == nil {
		return http.DefaultClient
	}
	c.noTimeoutOnce.Do(func() {
		cloned := *c.httpClient
		cloned.Timeout = 0
		c.noTimeoutClient = &cloned
	})
	return c.noTimeoutClient
}

func NewGuard(resolver Resolver, opts Options) (*Guard, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	blocked, err := parsePrefixes(defaultBlockedCIDRs())
	if err != nil {
		return nil, err
	}
	if len(opts.BlockedCIDRs) > 0 {
		extra, err := parsePrefixes(opts.BlockedCIDRs)
		if err != nil {
			return nil, err
		}
		blocked = append(blocked, extra...)
	}
	return &Guard{resolver: resolver, blockedRanges: blocked}, nil
}
func SanitizeHTTPURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, &ValidationError{Reason: "invalid url"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, &ValidationError{Reason: "invalid scheme"}
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, &ValidationError{Reason: "missing host"}
	}
	return parsed, nil
}
func (g *Guard) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := SanitizeHTTPURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := g.ValidateHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}
func (g *Guard) ValidateForPlatform(ctx context.Context, rawURL, platform string) (*url.URL, error) {
	return g.Validate(ctx, rawURL)
}
func (g *Guard) ValidateHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" || isLocalhostHost(host) {
		return &ValidationError{Reason: "blocked host"}
	}
	if isBlockedMirrorWorkerHost(host) {
		return &ValidationError{Reason: "blocked mirror worker host"}
	}
	addrs, err := g.resolveHost(ctx, host)
	if err != nil {
		return &ValidationError{Reason: "unable to resolve host"}
	}
	for _, addr := range addrs {
		if g.isBlocked(addr) {
			return &ValidationError{Reason: "blocked destination"}
		}
	}
	return nil
}

const (
	defaultHTTPTimeout = 30 * time.Second

	// dialTimeout is the maximum time to establish a TCP connection.
	dialTimeout = 10 * time.Second
	// keepAliveTimeout is the interval for TCP keep-alive probes.
	keepAliveTimeout = 30 * time.Second
	// tlsHandshakeTimeout is the maximum time to complete a TLS handshake.
	tlsHandshakeTimeout = 10 * time.Second
	// expectContinueTimeout is the time to wait for a 100-continue response before sending the request body.
	expectContinueTimeout = 1 * time.Second

	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 10
)

var (
	defaultHTTPClientOnce sync.Once
	defaultHTTPClientInst *http.Client
)

// NewDefaultHTTPClient returns a shared singleton HTTP client with connection pooling.
// Safe for concurrent use; connections are reused across calls.
func NewDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClientInst = &http.Client{
			Timeout: defaultHTTPTimeout,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAliveTimeout}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          defaultMaxIdleConns,
				MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   tlsHandshakeTimeout,
				ExpectContinueTimeout: expectContinueTimeout,
				DisableKeepAlives:     false,
			},
		}
	})
	return defaultHTTPClientInst
}

func NewHTTPClient(guard *Guard, opts HTTPClientOptions) *Client {
	httpClient := opts.Client
	var transport *http.Transport
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		idleConnTimeout := opts.IdleConnTimeout
		if idleConnTimeout <= 0 {
			idleConnTimeout = 90 * time.Second
		}
		maxIdleConns := opts.MaxIdleConns
		if maxIdleConns <= 0 {
			maxIdleConns = 128
		}
		maxConnsPerHost := opts.MaxConnsPerHost
		if maxConnsPerHost <= 0 {
			maxConnsPerHost = 32
		}
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAliveTimeout}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxConnsPerHost,
			MaxConnsPerHost:       maxConnsPerHost,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		}
		httpClient = &http.Client{Timeout: timeout, Transport: transport}
	} else if existingTransport, ok := httpClient.Transport.(*http.Transport); ok {
		transport = existingTransport.Clone()
	} else if httpClient.Transport == nil {
		transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAliveTimeout}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          defaultMaxIdleConns,
			MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
		}
	}
	if guard != nil {
		if transport == nil {
			transport = &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAliveTimeout}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          defaultMaxIdleConns,
				MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   tlsHandshakeTimeout,
				ExpectContinueTimeout: expectContinueTimeout,
			}
		}
		transport.DialContext = guardedDialContext(guard, transport.DialContext)

		cloned := *httpClient
		cloned.Transport = transport
		cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if err := guard.ValidateHost(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		httpClient = &cloned
	}
	return &Client{httpClient: httpClient, guard: guard}
}

func guardedDialContext(guard *Guard, base func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	if base == nil {
		base = (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAliveTimeout}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := base(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if isConnRemoteBlocked(guard, conn.RemoteAddr()) {
			_ = conn.Close()
			return nil, &ValidationError{Reason: "blocked destination"}
		}
		return conn, nil
	}
}

func isConnRemoteBlocked(guard *Guard, remoteAddr net.Addr) bool {
	if guard == nil || remoteAddr == nil {
		return false
	}
	host := remoteAddr.String()
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	return guard.isBlocked(addr.Unmap())
}
func (c *Client) Get(ctx context.Context, rawURL string, headers map[string]string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target := rawURL
	if c.guard != nil {
		parsed, err := c.guard.Validate(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		target = parsed.String()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return c.httpClient.Do(req)
}

func (c *Client) ValidateURL(ctx context.Context, rawURL string) error {
	if c == nil || c.guard == nil {
		return nil
	}
	_, err := c.guard.Validate(ctx, rawURL)
	return err
}
func (g *Guard) resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if parsed := net.ParseIP(host); parsed != nil {
		if addr, ok := netip.AddrFromSlice(parsed); ok {
			return []netip.Addr{addr.Unmap()}, nil
		}
	}
	results, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.New("empty dns answer")
	}
	addrs := make([]netip.Addr, 0, len(results))
	for _, result := range results {
		if addr, ok := netip.AddrFromSlice(result.IP); ok {
			addrs = append(addrs, addr.Unmap())
		}
	}
	if len(addrs) == 0 {
		return nil, errors.New("empty dns answer")
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].String() < addrs[j].String() })
	return addrs, nil
}
func (g *Guard) isBlocked(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	for _, prefix := range g.blockedRanges {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
func parsePrefixes(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", value, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}
func isLocalhostHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	return lower == "localhost" || strings.HasSuffix(lower, ".localhost")
}

func isBlockedMirrorWorkerHost(host string) bool {
	lower := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return lower == "workers.dev" || strings.HasSuffix(lower, ".workers.dev")
}

func defaultBlockedCIDRs() []string {
	return []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32", "::/128", "::1/128", "fe80::/10", "fc00::/7", "ff00::/8", "2001:db8::/32"}
}
