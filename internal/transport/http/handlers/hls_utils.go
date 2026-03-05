package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	infrahls "downaria-api/internal/infra/hls"
)

func isHLSPlaylist(rawURL, contentType string) bool {
	return infrahls.IsHLSPlaylist(rawURL, contentType)
}

func resolveURL(uri, baseURL string) string {
	return infrahls.ResolveURL(uri, baseURL)
}

func parseChunkParam(values url.Values) bool {
	v := strings.TrimSpace(values.Get("chunk"))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err == nil {
		return b
	}
	return v == "1"
}

func buildHLSProxyURL(routePath, absURL string, chunk bool) string {
	if chunk {
		return fmt.Sprintf("%s?url=%s&chunk=1", routePath, url.QueryEscape(absURL))
	}
	return fmt.Sprintf("%s?url=%s", routePath, url.QueryEscape(absURL))
}
