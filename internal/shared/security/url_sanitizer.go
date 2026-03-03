package security

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// SanitizeHTTPURL normalizes and validates a strict outbound http/https URL.
func SanitizeHTTPURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("missing url")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, errors.New("invalid url")
	}

	parsed.Scheme = strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("unsupported scheme")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil, errors.New("missing host")
	}

	if parsed.User != nil {
		return nil, errors.New("userinfo is not allowed")
	}

	port := strings.TrimSpace(parsed.Port())
	if port != "" {
		n, parseErr := strconv.Atoi(port)
		if parseErr != nil || n < 1 || n > 65535 {
			return nil, errors.New("invalid port")
		}
	}

	host = strings.ToLower(host)
	if port != "" {
		parsed.Host = net.JoinHostPort(host, port)
	} else {
		parsed.Host = host
	}

	parsed.Fragment = ""

	if parsed.Opaque != "" {
		return nil, fmt.Errorf("invalid opaque url")
	}

	return parsed, nil
}

func SanitizeHTTPURLString(raw string) (string, error) {
	parsed, err := SanitizeHTTPURL(raw)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}
