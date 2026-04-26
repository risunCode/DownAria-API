package facebook

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"downaria-api/internal/extract"
)

func extractInlineThumbnail(htmlStr string) string {
	for _, pattern := range fbInlineThumbnailPatterns {
		if match := pattern.FindStringSubmatch(htmlStr); len(match) > 1 {
			thumb := strings.TrimSpace(unescapeURL(match[1]))
			if thumb != "" {
				return thumb
			}
		}
	}
	return ""
}

func normalizeQualityForDedup(quality string) string {
	q := strings.ToUpper(strings.TrimSpace(quality))
	if q == "" {
		return "ORIGINAL"
	}
	return q
}

func canonicalURLForDedup(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return strings.TrimSpace(raw)
	}
	return strings.ToLower(parsed.Host) + parsed.EscapedPath()
}

func unescapeURL(raw string) string {
	value := raw
	replacements := map[string]string{`\/`: `/`, `\u003A`: `:`, `\u002F`: `/`, `\u003B`: `;`, `\u003D`: `=`, `\u0026`: `&`, `\u003F`: `?`, `\u0025`: `%`, `\u0023`: `#`, `\u0024`: `$`, `\u0040`: `@`, `&quot;`: `"`, `&amp;`: `&`, `&lt;`: `<`, `&gt;`: `>`, `&#x3D;`: `=`, `&#x2F;`: `/`}
	for old, newValue := range replacements {
		value = strings.ReplaceAll(value, old, newValue)
	}
	return value
}

func normalizeTextField(raw string) string {
	value := strings.TrimSpace(raw)
	value = unescapeHTML(value)
	value = decodeJSONStringEscapes(value)
	return strings.TrimSpace(value)
}

func parseStatsFromTitleLikeText(text string) (int64, int64) {
	views := int64(0)
	likes := int64(0)
	if match := fbTitleViewsPattern.FindStringSubmatch(text); len(match) > 1 {
		views = extract.ParseHumanInt(strings.ReplaceAll(match[1], ",", ""))
	}
	if match := fbTitleLikesPattern.FindStringSubmatch(text); len(match) > 1 {
		likes = extract.ParseHumanInt(strings.ReplaceAll(match[1], ",", ""))
	}
	return views, likes
}

func cleanFacebookTitle(title, author string) string {
	clean := strings.TrimSpace(title)
	if clean == "" {
		return ""
	}
	clean = fbTitleSeparatorRegex.ReplaceAllString(clean, ` $1 `)
	clean = strings.Join(strings.Fields(clean), " ")
	clean = fbTitleStatsPrefixRegex.ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)
	if author != "" {
		if idx := strings.LastIndex(clean, "|"); idx >= 0 {
			suffix := strings.TrimSpace(clean[idx+1:])
			if suffix != "" && strings.EqualFold(suffix, strings.TrimSpace(author)) {
				clean = strings.TrimSpace(clean[:idx])
			}
		}
	}
	clean = fbTitleSeparatorRegex.ReplaceAllString(clean, ` $1 `)
	clean = strings.Join(strings.Fields(clean), " ")
	return strings.TrimSpace(clean)
}

func extractFirstAuthor(htmlStr string, patterns []*regexp.Regexp, validator func(string) bool) string {
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(htmlStr); len(match) > 1 {
			author := normalizeTextField(match[1])
			if validator(author) {
				return author
			}
		}
	}
	return ""
}

func isUsableGenericAuthor(author string) bool {
	a := strings.TrimSpace(author)
	return a != "" && !strings.EqualFold(a, "facebook")
}

func isUsableStoryAuthor(author string) bool {
	a := strings.TrimSpace(author)
	if a == "" {
		return false
	}
	lower := strings.ToLower(a)
	if lower == "facebook" || lower == "story" || lower == "stories" || lower == "profile.php" || lower == "permalink" || lower == "watch" {
		return false
	}
	return !fbNumericRe.MatchString(lower)
}

func extractStoryAuthorFromURL(finalURL string) string {
	u, err := url.Parse(finalURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(segments); i++ {
		if strings.EqualFold(segments[i], "stories") && i+1 < len(segments) {
			candidate, unescapeErr := url.PathUnescape(strings.TrimSpace(segments[i+1]))
			if unescapeErr != nil {
				candidate = strings.TrimSpace(segments[i+1])
			}
			if isUsableStoryAuthor(candidate) {
				return normalizeTextField(candidate)
			}
		}
	}
	return ""
}

func extractCreatedAt(htmlStr string) string {
	for _, pattern := range fbCreatedAtNumericPatterns {
		if match := pattern.FindStringSubmatch(htmlStr); len(match) > 1 {
			if ts := normalizeCreatedAt(match[1]); ts != "" {
				return ts
			}
		}
	}
	for _, pattern := range fbCreatedAtStringPatterns {
		if match := pattern.FindStringSubmatch(htmlStr); len(match) > 1 {
			if ts := normalizeCreatedAt(match[1]); ts != "" {
				return ts
			}
		}
	}
	return ""
}

func normalizeCreatedAt(raw string) string {
	v := strings.TrimSpace(unescapeHTML(raw))
	if v == "" {
		return ""
	}
	if fbNumericRe.MatchString(v) {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ""
		}
		if len(v) >= 13 {
			n /= 1000
		}
		if n <= 0 {
			return ""
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, v); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func unescapeHTML(raw string) string {
	value := html.UnescapeString(raw)
	hexPattern := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	value = hexPattern.ReplaceAllStringFunc(value, func(match string) string {
		hexStr := match[3 : len(match)-1]
		if code, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
			return string(rune(code))
		}
		return match
	})
	decPattern := regexp.MustCompile(`&#(\d+);`)
	value = decPattern.ReplaceAllStringFunc(value, func(match string) string {
		decStr := match[2 : len(match)-1]
		if code, err := strconv.ParseInt(decStr, 10, 32); err == nil {
			return string(rune(code))
		}
		return match
	})
	return value
}

func decodeJSONStringEscapes(raw string) string {
	if raw == "" || !strings.Contains(raw, `\`) {
		return raw
	}
	quoted := `"` + strings.ReplaceAll(raw, `"`, `\\"`) + `"`
	decoded, err := strconv.Unquote(quoted)
	if err != nil {
		return raw
	}
	return decoded
}

func isFacebookStoryURL(urlStr string) bool {
	return strings.Contains(strings.ToLower(urlStr), "/stories/")
}

func looksLikeVideoURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.Contains(lower, ".mp4") || strings.Contains(lower, ".m3u8") || strings.Contains(lower, "video")
}

func protocolForFBURL(rawURL string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(rawURL)), ".m3u8") {
		return "m3u8_native"
	}
	return "https"
}
