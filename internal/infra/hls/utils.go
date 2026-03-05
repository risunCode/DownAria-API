package hls

import (
	"net/url"
	"strings"
)

func IsHLSPlaylist(rawURL, contentType string) bool {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	ct := strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasSuffix(u, ".m3u8") ||
		strings.Contains(u, "m3u8") ||
		strings.Contains(u, "/manifest/") ||
		strings.Contains(u, "index.m3u8") ||
		strings.Contains(ct, "mpegurl")
}

func ResolveURL(uri, baseURL string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return uri
	}
	ref, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return base.ResolveReference(ref).String()
}
