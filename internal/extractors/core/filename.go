package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	filenameURLRe        = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
	filenameSpaceRe      = regexp.MustCompile(`\s+`)
	filenamePunctRe      = regexp.MustCompile(`[\[\]\(\){}<>:"'/\\|?*~` + "`" + `,;+=!$%^&@#]`)
	filenameAllowedRe    = regexp.MustCompile(`[^a-z0-9 _-]`)
	filenameUnderscoreRe = regexp.MustCompile(`_+`)
	extAllowedRe         = regexp.MustCompile(`^[a-z0-9]{2,8}$`)
	filenameNowFunc      = time.Now
)

const maxFilenameTitleWords = 15

// GenerateFilename creates a standardized filename: author_title_id_[DownAria].ext
// Format: {author}_{title}_{id}_[DownAria].{extension}
func GenerateFilename(author, title, contentID, extension string) string {
	// Sanitize author
	author = sanitizeFilenameComponent(author)
	if author == "" {
		author = "unknown"
	}

	// Sanitize title (optional)
	title = sanitizeFilenameComponent(title)
	title = capFilenameTitleWords(title, maxFilenameTitleWords)

	// Sanitize content ID (optional)
	contentID = sanitizeFilenameComponent(contentID)

	// Sanitize extension
	extension = sanitizeExtension(extension)

	// Build filename: author[_title][_id]_[DownAria].ext
	parts := []string{author}
	if title != "" {
		parts = append(parts, title)
	}
	if contentID != "" {
		parts = append(parts, contentID)
	}
	filename := fmt.Sprintf("%s_[DownAria].%s", strings.Join(parts, "_"), extension)

	// Ensure total length doesn't exceed 220 chars (safer across environments)
	if len(filename) > 220 {
		filename = truncateFilename(filename, 220)
	}

	return filename
}

// BuildFilenameID creates stable identifier segment.
// Preferred format:
//   - authorID_postID (when both exist)
//   - authorID_YYYYMMDDHHMMSS (when only authorID exists)
//   - postID (when only postID exists)
//   - YYYYMMDDHHMMSS (fallback)
func BuildFilenameID(authorID, postID string) string {
	aid := sanitizeFilenameComponent(authorID)
	pid := sanitizeFilenameComponent(postID)
	ts := filenameNowFunc().UTC().Format("20060102150405")

	switch {
	case aid != "" && pid != "":
		return aid + "_" + pid
	case aid != "":
		return aid + "_" + ts
	case pid != "":
		return pid
	default:
		return ts
	}
}

// GenerateFilenameWithMeta builds filename using author/title and ID strategy.
func GenerateFilenameWithMeta(author, title, authorID, postID, extension string) string {
	return GenerateFilename(author, title, BuildFilenameID(authorID, postID), extension)
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

	// Keep components reasonably short (UTF-8 safe)
	if len(s) > 80 {
		s = truncateUTF8(s, 80)
		s = strings.Trim(s, "_")
	}

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

func capFilenameTitleWords(v string, maxWords int) string {
	if v == "" || maxWords <= 0 {
		return ""
	}

	parts := strings.Split(v, "_")
	trimmed := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		trimmed = append(trimmed, p)
		if len(trimmed) >= maxWords {
			break
		}
	}

	if len(trimmed) == 0 {
		return ""
	}

	return strings.Join(trimmed, "_")
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
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))

	// Remove parameters (e.g., "video/mp4; codecs=..." -> "video/mp4")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = mimeType[:idx]
	}

	switch mimeType {
	// Video
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "video/mpeg":
		return "mpeg"
	case "video/x-msvideo":
		return "avi"
	case "video/x-matroska":
		return "mkv"

	// Audio
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4", "audio/x-m4a":
		return "m4a"
	case "audio/aac":
		return "aac"
	case "audio/ogg":
		return "ogg"
	case "audio/wav":
		return "wav"
	case "audio/webm":
		return "webm"
	case "audio/flac":
		return "flac"

	// Image
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "image/bmp":
		return "bmp"
	case "image/svg+xml":
		return "svg"

	default:
		return "bin"
	}
}
