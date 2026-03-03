package core

import (
	"fmt"
	"regexp"
	"strings"
)

// GenerateFilename creates a standardized filename: author_title_id_[DownAria].ext
// Format: {author}_{title}_{id}_[DownAria].{extension}
func GenerateFilename(author, title, contentID, extension string) string {
	// Sanitize author
	author = sanitizeFilenameComponent(author)
	if author == "" {
		author = "unknown"
	}

	// Sanitize title
	title = sanitizeFilenameComponent(title)
	if title == "" {
		title = "untitled"
	}

	// Sanitize content ID (optional)
	contentID = sanitizeFilenameComponent(contentID)

	// Build filename: author_title_id_[DownAria].ext
	var filename string
	if contentID != "" {
		filename = fmt.Sprintf("%s_%s_%s_[DownAria].%s", author, title, contentID, extension)
	} else {
		filename = fmt.Sprintf("%s_%s_[DownAria].%s", author, title, extension)
	}

	// Ensure total length doesn't exceed 250 chars (filesystem safe limit)
	if len(filename) > 250 {
		filename = truncateFilename(filename, 250)
	}

	return filename
}

// sanitizeFilenameComponent removes/replaces invalid filename characters
func sanitizeFilenameComponent(s string) string {
	if s == "" {
		return ""
	}

	// Convert to lowercase and trim spaces
	s = strings.ToLower(strings.TrimSpace(s))

	// Replace multiple spaces with single space
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")

	// Replace spaces with underscores
	s = strings.ReplaceAll(s, " ", "_")

	// Remove invalid filename characters: < > : " / \ | ? *
	// Also remove emoji and special unicode
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*]`)
	s = invalidChars.ReplaceAllString(s, "")

	// Remove consecutive underscores
	s = regexp.MustCompile(`_+`).ReplaceAllString(s, "_")

	// Remove leading/trailing underscores
	s = strings.Trim(s, "_")

	return s
}

// truncateFilename safely truncates filename while preserving extension
func truncateFilename(filename string, maxLen int) string {
	if len(filename) <= maxLen {
		return filename
	}

	// Find last dot (extension)
	lastDot := strings.LastIndex(filename, ".")
	if lastDot == -1 {
		// No extension, just truncate
		return filename[:maxLen]
	}

	ext := filename[lastDot:]
	extLen := len(ext)
	availableLen := maxLen - extLen

	if availableLen <= 0 {
		// Extension is too long, just truncate everything
		return filename[:maxLen]
	}

	base := filename[:lastDot]
	base = base[:availableLen]
	base = strings.TrimRight(base, "_")

	return base + ext
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
