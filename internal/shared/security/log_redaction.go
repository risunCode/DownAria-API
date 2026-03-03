package security

import (
	"net/url"
	"regexp"
	"strings"
)

var sensitiveQueryValueRe = regexp.MustCompile(`(?i)([?&](?:access_token|refresh_token|token|auth|authorization|upstream_auth|apikey|api_key|key|signature|sig|cookie|session|password|passwd|secret|jwt)=)([^&\s]+)`)
var sensitiveHeaderValueRe = regexp.MustCompile(`(?i)((?:authorization|proxy-authorization|x-upstream-authorization|cookie|set-cookie)\s*[:=]\s*)([^\r\n]+)`)
var sensitiveBearerTokenRe = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[a-z0-9._~+\-/]+=*`)
var absoluteURLRe = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

func RedactLogValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	trimmed = redactURLQueriesInText(trimmed)
	trimmed = sensitiveQueryValueRe.ReplaceAllString(trimmed, `${1}[REDACTED]`)
	trimmed = sensitiveHeaderValueRe.ReplaceAllString(trimmed, `${1}[REDACTED]`)
	trimmed = sensitiveBearerTokenRe.ReplaceAllString(trimmed, `${1} [REDACTED]`)

	return trimmed
}

func RedactLogError(err error) string {
	if err == nil {
		return ""
	}
	return RedactLogValue(err.Error())
}

func redactURLQueriesInText(input string) string {
	return absoluteURLRe.ReplaceAllStringFunc(input, func(candidate string) string {
		suffix := ""
		for len(candidate) > 0 {
			last := candidate[len(candidate)-1]
			if strings.ContainsRune(",.;:!?)]}", rune(last)) {
				suffix = string(last) + suffix
				candidate = candidate[:len(candidate)-1]
				continue
			}
			break
		}

		parsed, err := url.Parse(candidate)
		if err != nil || parsed == nil {
			return candidate + suffix
		}

		parsed.RawQuery = ""
		parsed.Fragment = ""
		if parsed.User != nil {
			parsed.User = url.User("[REDACTED]")
		}

		return parsed.String() + suffix
	})
}
