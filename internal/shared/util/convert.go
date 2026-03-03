package util

import (
	"regexp"
	"strconv"
	"strings"
)

func ParseInt64OrZero(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0
	}

	return parsed
}

func ParseIntOrDefault(value string, fallback int) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return fallback
	}

	return parsed
}

func ExtractLeadingDigitsIntOrZero(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	var digits strings.Builder
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			break
		}
		digits.WriteRune(char)
	}

	if digits.Len() == 0 {
		return 0
	}

	return ParseIntOrDefault(digits.String(), 0)
}

func ClampNonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}

	return value
}

func ExtractFirstRegexGroup(value string, re *regexp.Regexp) string {
	if re == nil {
		return ""
	}

	matches := re.FindStringSubmatch(value)
	if len(matches) <= 1 {
		return ""
	}

	return matches[1]
}
