package ytdlp

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type limitedBuffer struct {
	buf        bytes.Buffer
	limit      int
	overflowed bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.overflowed {
		return len(p), nil
	}
	if b.limit > 0 && b.buf.Len()+len(p) > b.limit {
		allowed := b.limit - b.buf.Len()
		if allowed > 0 {
			_, _ = b.buf.Write(p[:allowed])
		}
		b.overflowed = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func validateBinaryPath(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("binary path is empty")
	}
	return nil
}

func isUnsupportedError(stderr string, err error) bool {
	if errors.Is(err, nil) {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(stderr))
	return strings.Contains(lower, "unsupported url") || strings.Contains(lower, "unsupported")
}

func sanitizeCookieHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", ";")
	return strings.TrimSpace(value)
}

func sanitizeCookieToken(value string) string {
	if strings.ContainsAny(value, "\r\n\t") {
		return ""
	}
	value = strings.TrimSpace(value)
	return value
}

func sanitizeCookieValue(value string) string {
	if strings.ContainsAny(value, "\r\n") {
		return ""
	}
	value = strings.TrimSpace(value)
	return value
}

func cookieDomain(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return ""
	}
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	if strings.Count(host, ".") == 0 {
		return host
	}
	return "." + host
}

func sourceReferer(dump *Dump) string {
	if dump == nil {
		return ""
	}
	return extractFirstNonEmpty(strings.TrimSpace(dump.WebpageURL), strings.TrimSpace(dump.OriginalURL))
}

func extractFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
