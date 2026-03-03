package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
)

type ipLookupResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type OutboundURLValidator struct {
	resolver      ipLookupResolver
	blockedRanges []netip.Prefix
}

func NewOutboundURLValidator(resolver ipLookupResolver) *OutboundURLValidator {
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	blocked := mustBlockedPrefixes([]string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"255.255.255.255/32",
		"::/128",
		"::1/128",
		"fe80::/10",
		"fc00::/7",
		"ff00::/8",
		"2001:db8::/32",
	})

	return &OutboundURLValidator{resolver: resolver, blockedRanges: blocked}
}

func (v *OutboundURLValidator) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	if v == nil {
		return nil, errors.New("validator is nil")
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, errors.New("invalid url")
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("unsupported scheme")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil, errors.New("missing host")
	}

	if isLocalhostHost(host) {
		return nil, errors.New("blocked host")
	}

	addresses, err := v.resolveHost(ctx, host)
	if err != nil {
		return nil, errors.New("unable to resolve host")
	}

	for _, addr := range addresses {
		if v.isBlocked(addr) {
			return nil, errors.New("blocked destination")
		}
	}

	return parsed, nil
}

func (v *OutboundURLValidator) resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if parsed := net.ParseIP(host); parsed != nil {
		if addr, ok := netip.AddrFromSlice(parsed); ok {
			return []netip.Addr{addr.Unmap()}, nil
		}
	}

	results, err := v.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.New("empty dns answer")
	}

	addrs := make([]netip.Addr, 0, len(results))
	for _, result := range results {
		addr, ok := netip.AddrFromSlice(result.IP)
		if !ok {
			continue
		}
		addrs = append(addrs, addr.Unmap())
	}

	if len(addrs) == 0 {
		return nil, errors.New("empty dns answer")
	}

	sort.Slice(addrs, func(i, j int) bool {
		return addrs[i].String() < addrs[j].String()
	})

	return addrs, nil
}

func (v *OutboundURLValidator) isBlocked(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	for _, prefix := range v.blockedRanges {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func isLocalhostHost(host string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))
	if lower == "localhost" {
		return true
	}
	return strings.HasSuffix(lower, ".localhost")
}

func mustBlockedPrefixes(values []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			panic(fmt.Sprintf("invalid blocked prefix %q: %v", value, err))
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}
