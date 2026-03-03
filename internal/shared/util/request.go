package util

import (
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
)

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type IPAllowlist struct {
	prefixes []netip.Prefix
}

func NewIPAllowlist(values []string) (*IPAllowlist, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		if strings.Contains(trimmed, "/") {
			prefix, err := netip.ParsePrefix(trimmed)
			if err != nil {
				return nil, err
			}
			prefixes = append(prefixes, prefix)
			continue
		}

		addr, err := netip.ParseAddr(trimmed)
		if err != nil {
			return nil, err
		}
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr.Unmap(), bits))
	}

	sort.Slice(prefixes, func(i, j int) bool {
		return prefixes[i].String() < prefixes[j].String()
	})

	if len(prefixes) == 0 {
		return nil, nil
	}

	return &IPAllowlist{prefixes: prefixes}, nil
}

func (a *IPAllowlist) Contains(addr netip.Addr) bool {
	if a == nil || !addr.IsValid() {
		return false
	}
	unmapped := addr.Unmap()
	for _, prefix := range a.prefixes {
		if prefix.Contains(unmapped) {
			return true
		}
	}
	return false
}

func ClientIPFromRequest(r *http.Request) string {
	return ClientIPFromRequestWithTrustedProxies(r, nil)
}

func ClientIPFromRequestWithTrustedProxies(r *http.Request, trustedProxies *IPAllowlist) string {
	if r == nil {
		return ""
	}

	remoteIP := parseIPCandidate(r.RemoteAddr)

	if remoteIP.IsValid() && trustedProxies != nil && trustedProxies.Contains(remoteIP) {
		if forwardedIP := parseFirstForwardedIP(r.Header.Get("X-Forwarded-For")); forwardedIP.IsValid() {
			return forwardedIP.String()
		}
		if realIP := parseIPCandidate(r.Header.Get("X-Real-IP")); realIP.IsValid() {
			return realIP.String()
		}
	}

	if remoteIP.IsValid() {
		return remoteIP.String()
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func parseFirstForwardedIP(value string) netip.Addr {
	parts := strings.Split(strings.TrimSpace(value), ",")
	for _, part := range parts {
		if ip := parseIPCandidate(part); ip.IsValid() {
			return ip
		}
	}
	return netip.Addr{}
}

func parseIPCandidate(value string) netip.Addr {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return netip.Addr{}
	}

	if addr, err := netip.ParseAddr(trimmed); err == nil {
		return addr.Unmap()
	}

	host, _, err := net.SplitHostPort(trimmed)
	if err == nil {
		if addr, parseErr := netip.ParseAddr(strings.TrimSpace(host)); parseErr == nil {
			return addr.Unmap()
		}
	}

	return netip.Addr{}
}
