package netutil

import (
	"net/url"
	"strings"
)

func HostFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func SameHost(a, b string) bool {
	hostA := HostFromURL(a)
	hostB := HostFromURL(b)
	return hostA != "" && hostB != "" && hostA == hostB
}

func CookieForSameHost(cookieHeader, urlA, urlB string) string {
	if strings.TrimSpace(cookieHeader) == "" {
		return ""
	}
	if !SameHost(urlA, urlB) {
		return ""
	}
	return cookieHeader
}
