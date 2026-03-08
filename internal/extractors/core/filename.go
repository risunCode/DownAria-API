package core

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	mediapkg "downaria-api/pkg/media"
)

var (
	filenameURLRe        = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
	filenameSpaceRe      = regexp.MustCompile(`\s+`)
	filenamePunctRe      = regexp.MustCompile(`[\[\]\(\){}<>:"'/\\|?*~` + "`" + `,;+=!$%^&@#]`)
	filenameAllowedRe    = regexp.MustCompile(`[^\p{L}\p{N} _-]`)
	filenameUnderscoreRe = regexp.MustCompile(`_+`)
	extAllowedRe         = regexp.MustCompile(`^[a-z0-9]{2,8}$`)
)

// GenerateFilename creates a standardized filename: author_title[_index]_[DownAria].ext
// Format: {author}_{title}_[{index}_][DownAria].{extension}
func GenerateFilename(author, title, contentID, extension string) string {
	// Sanitize author
	author = sanitizeFilenameComponent(author)
	if author == "" {
		author = "unknown"
	}

	// Sanitize title (optional)
	title = sanitizeFilenameComponent(title)
	if title == "" {
		title = "media"
	}

	indexToken := sanitizeFilenameComponent(contentID)
	if indexToken == "0" {
		indexToken = ""
	}

	// Sanitize extension
	extension = sanitizeExtension(extension)

	// Build filename: author_title[_index]_[DownAria].ext
	if indexToken != "" {
		return fmt.Sprintf("%s_%s_%s_[DownAria].%s", author, title, indexToken, extension)
	}
	filename := fmt.Sprintf("%s_%s_[DownAria].%s", author, title, extension)

	return filename
}

// sanitizeFilenameComponent removes/replaces invalid filename characters
func sanitizeFilenameComponent(s string) string {
	if s == "" {
		return ""
	}

	// Convert to lowercase and trim spaces
	s = strings.ToLower(strings.TrimSpace(s))

	// Remove URLs first (very noisy in captions)
	s = filenameURLRe.ReplaceAllString(s, " ")

	// Normalize common separators to spaces
	s = filenamePunctRe.ReplaceAllString(s, " ")

	// Keep only safe ascii symbols (drop emoji/unsupported unicode/mojibake)
	s = filenameAllowedRe.ReplaceAllString(s, "")

	// Replace multiple spaces with single space
	s = filenameSpaceRe.ReplaceAllString(s, " ")

	// Replace spaces with underscores
	s = strings.ReplaceAll(s, " ", "_")

	// Remove consecutive underscores
	s = filenameUnderscoreRe.ReplaceAllString(s, "_")

	// Remove leading/trailing underscores
	s = strings.Trim(s, "_")

	return s
}

func sanitizeExtension(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	ext = strings.TrimPrefix(ext, ".")
	if !extAllowedRe.MatchString(ext) {
		return "bin"
	}
	return ext
}

// truncateFilename safely truncates filename while preserving extension
func truncateFilename(filename string, maxLen int) string {
	if len(filename) <= maxLen {
		return filename
	}

	// Find last dot (extension)
	lastDot := strings.LastIndex(filename, ".")
	if lastDot == -1 {
		return truncateUTF8(filename, maxLen)
	}

	ext := filename[lastDot:]
	extLen := len(ext)
	availableLen := maxLen - extLen

	if availableLen <= 0 {
		return truncateUTF8(filename, maxLen)
	}

	base := truncateUTF8(filename[:lastDot], availableLen)
	base = strings.TrimRight(base, "_")

	return base + ext
}

// truncateUTF8 safely truncates a string to maxBytes without breaking multi-byte runes
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return strings.TrimSpace(s[:maxBytes])
}

// GetExtensionFromMime returns file extension for a given MIME type
func GetExtensionFromMime(mimeType string) string {
	return mediapkg.GetExtensionFromMime(mimeType)
}
