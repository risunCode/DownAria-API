package handlers

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"strings"
)

// rewriteHLSPlaylistForRoute rewrites HLS playlist URLs to proxy through a specific route
func rewriteHLSPlaylistForRoute(content []byte, originalURL, proxyBaseURL, routePath string) ([]byte, error) {
	parsedOriginal, err := url.Parse(originalURL)
	if err != nil {
		return content, err
	}

	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			if strings.Contains(line, "URI=") {
				line = rewriteURIAttributeForRoute(line, parsedOriginal, proxyBaseURL, routePath)
			}
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		if strings.TrimSpace(line) == "" {
			result.WriteString("\n")
			continue
		}

		rewrittenURI := rewritePlaylistURIForRoute(line, parsedOriginal, proxyBaseURL, routePath)
		result.WriteString(rewrittenURI)
		result.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return content, err
	}

	return result.Bytes(), nil
}

func rewritePlaylistURIForRoute(uri string, originalURL *url.URL, proxyBaseURL, routePath string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return uri
	}
	resolvedURL := resolvePlaylistURL(uri, originalURL)
	return fmt.Sprintf("%s%s?url=%s", proxyBaseURL, routePath, url.QueryEscape(resolvedURL))
}

func rewriteURIAttributeForRoute(line string, originalURL *url.URL, proxyBaseURL, routePath string) string {
	uriStart := strings.Index(line, `URI="`)
	if uriStart == -1 {
		return line
	}

	uriStart += 5
	uriEnd := strings.Index(line[uriStart:], `"`)
	if uriEnd == -1 {
		return line
	}

	uri := line[uriStart : uriStart+uriEnd]
	resolvedURL := resolvePlaylistURL(uri, originalURL)
	rewrittenURI := fmt.Sprintf("%s%s?url=%s", proxyBaseURL, routePath, url.QueryEscape(resolvedURL))

	return line[:uriStart] + rewrittenURI + line[uriStart+uriEnd:]
}

// resolvePlaylistURL resolves relative/absolute URIs to full URLs
func resolvePlaylistURL(uri string, baseURL *url.URL) string {
	uri = strings.TrimSpace(uri)

	// Already a full URL
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}

	// Absolute path (starts with /)
	if strings.HasPrefix(uri, "/") {
		return fmt.Sprintf("%s://%s%s", baseURL.Scheme, baseURL.Host, uri)
	}

	// Relative path - resolve against base URL directory
	baseDir := baseURL.String()
	if lastSlash := strings.LastIndex(baseDir, "/"); lastSlash > 8 { // After "https://"
		baseDir = baseDir[:lastSlash+1]
	}
	return baseDir + uri
}

// isHLSPlaylist checks if content type indicates HLS playlist
func isHLSPlaylist(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	return strings.Contains(ct, "application/vnd.apple.mpegurl") ||
		strings.Contains(ct, "application/x-mpegurl") ||
		strings.Contains(ct, "audio/mpegurl") ||
		strings.Contains(ct, "audio/x-mpegurl")
}
