package extract

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"mime"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	filenameURLRe        = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
	filenameSpaceRe      = regexp.MustCompile(`\s+`)
	filenamePunctRe      = regexp.MustCompile(`[\(\){}<>:"'/\\|?*~` + "`" + `,;+=!$%^&@#]`)
	filenameUnderscoreRe = regexp.MustCompile(`_+`)
	extAllowedRe         = regexp.MustCompile(`^[a-z0-9]{2,8}$`)
)

var windowsReservedNames = map[string]struct{}{"con": {}, "prn": {}, "aux": {}, "nul": {}, "com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {}, "com6": {}, "com7": {}, "com8": {}, "com9": {}, "lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {}, "lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {}}

func applyFilename(result *Result) {
	if result == nil {
		return
	}
	ext := inferExtension(result)
	base := buildFilenameBase(result)
	if ext == "" {
		result.Filename = base
		applyMediaFilenames(result, base, "")
		return
	}
	result.Filename = base + "." + ext
	applyMediaFilenames(result, base, ext)
}

func applyMediaFilenames(result *Result, base string, ext string) {
	if result == nil || len(result.Media) == 0 {
		return
	}
	if len(result.Media) == 1 {
		result.Media[0].Filename = result.Filename
		return
	}
	width := len(strconv.Itoa(len(result.Media)))
	if width < 2 {
		width = 2
	}
	for i := range result.Media {
		suffix := "_" + fmt.Sprintf("%0*d", width, i+1)
		if ext == "" {
			result.Media[i].Filename = base + suffix
			continue
		}
		result.Media[i].Filename = base + suffix + "." + ext
	}
}

func buildFilenameBase(result *Result) string {
	handle := sanitizeFilenameComponent(FirstNonEmpty(result.Author.Handle, result.Author.Name, result.Platform, "unknown"))
	title := sanitizeFilenameComponent(normalizeTitleForFilename(FirstNonEmpty(result.Title, result.ContentType, "media")))
	if handle == "" {
		handle = "unknown"
	}
	if title == "" {
		title = "media"
	}
	title = truncateComponent(title, 72)
	seed := FirstNonEmpty(result.SourceURL, result.Platform+":"+title)
	hash := shortHash(seed)
	parts := []string{handle}
	parts = append(parts, title, hash, "[DownAria]")
	base := strings.Trim(strings.Join(parts, "_"), "_")
	return truncateFilename(base, 120)
}

func normalizeTitleForFilename(value string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
	if clean == "" {
		return clean
	}
	if hashIndex := strings.Index(clean, "#"); hashIndex > 0 {
		clean = strings.TrimSpace(clean[:hashIndex])
	}
	clean = filenameSpaceRe.ReplaceAllString(clean, " ")
	return clean
}

func inferExtension(result *Result) string {
	if result == nil {
		return ""
	}
	if ext := preferredVideoExtension(result.Media); ext != "" {
		return ext
	}
	for _, item := range primaryMediaItems(result.Media) {
		for _, source := range item.Sources {
			if ext := sanitizeExtension(FirstNonEmpty(source.Container, extensionFromMIME(source.MIMEType), extensionFromURL(source.URL))); ext != "bin" {
				return ext
			}
		}
	}
	return sanitizeExtension(extensionFromContentType(result.ContentType))
}

func preferredVideoExtension(items []MediaItem) string {
	for _, item := range primaryMediaItems(items) {
		if strings.ToLower(strings.TrimSpace(item.Type)) != "video" {
			continue
		}
		for _, preferred := range []string{"mp4", "m4v", "mov", "webm"} {
			for _, source := range item.Sources {
				ext := sanitizeExtension(FirstNonEmpty(source.Container, extensionFromMIME(source.MIMEType), extensionFromURL(source.URL)))
				if ext == preferred {
					return ext
				}
			}
		}
	}
	return ""
}

func sanitizeFilenameComponent(s string) string {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	s = filenameURLRe.ReplaceAllString(s, " ")
	s = filenamePunctRe.ReplaceAllString(s, " ")
	s = stripUnsafeFilenameRunes(s, true)
	s = filenameSpaceRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, " ", "_")
	s = filenameUnderscoreRe.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func SanitizeFilename(name string, fallback string) string {
	value := strings.TrimSpace(name)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = filenamePunctRe.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, "*", "_")
	value = strings.ReplaceAll(value, "?", "_")
	value = strings.ReplaceAll(value, "\"", "_")
	value = strings.ReplaceAll(value, "<", "_")
	value = strings.ReplaceAll(value, ">", "_")
	value = strings.ReplaceAll(value, "|", "_")
	value = stripUnsafeFilenameRunes(value, true)
	value = filenameSpaceRe.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, " ", "_")
	value = filenameUnderscoreRe.ReplaceAllString(value, "_")
	value = strings.ReplaceAll(value, "_._", ".")
	value = strings.ReplaceAll(value, "_.", ".")
	value = strings.Trim(value, " ._")
	value = ensureSafeFilenameBase(value)
	if value == "" {
		return "download.bin"
	}
	return trimFilenameLength(value, 180)
}

func trimFilenameLength(name string, max int) string {
	if max <= 0 {
		return name
	}
	if len([]rune(name)) <= max {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if ext == "" {
		return string([]rune(name)[:max])
	}
	extRunes := []rune(ext)
	if len(extRunes) >= max {
		return string(append([]rune{}, extRunes[:max]...))
	}
	baseRunes := []rune(base)
	allowedBase := max - len(extRunes)
	if allowedBase <= 0 {
		return string(append([]rune{}, extRunes[:max]...))
	}
	if len(baseRunes) > allowedBase {
		baseRunes = baseRunes[:allowedBase]
	}
	return string(append(baseRunes, extRunes...))
}

func EnsureFilenameExtension(name, extension string) string {
	base := strings.TrimSpace(strings.TrimSuffix(name, filepath.Ext(name)))
	if base == "" {
		base = "output"
	}
	ext := sanitizeExtension(extension)
	if ext == "bin" {
		return base
	}
	return base + "." + ext
}

func AttachmentDisposition(filename string) string {
	filename = SanitizeFilename(filename, "download.bin")
	asciiFallback := asciiFilenameFallback(filename)
	params := map[string]string{"filename": asciiFallback}
	value := mime.FormatMediaType("attachment", params)
	if strings.TrimSpace(value) == "" {
		value = `attachment; filename="download.bin"`
	}
	encoded := encodeRFC5987(filename)
	if encoded != "" && filename != asciiFallback {
		value += "; filename*=UTF-8''" + encoded
	}
	return value
}

func stripUnsafeFilenameRunes(value string, stripEmoji bool) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == 0:
			continue
		case r < 32 || r == 127:
			continue
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r):
			builder.WriteRune(r)
		case r == ' ' || r == '_' || r == '-' || r == '.' || r == '[' || r == ']':
			builder.WriteRune(r)
		case !stripEmoji && (unicode.IsSymbol(r) || unicode.IsPunct(r)):
			builder.WriteRune(r)
		case unicode.IsPunct(r), unicode.IsSymbol(r):
			builder.WriteRune(' ')
		default:
			builder.WriteRune(' ')
		}
	}
	return builder.String()
}

func asciiFilenameFallback(filename string) string {
	filename = strings.ToValidUTF8(strings.TrimSpace(filename), "")
	var builder strings.Builder
	for _, r := range filename {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.', r == '_', r == '-', r == '[', r == ']':
			builder.WriteRune(r)
		case unicode.IsSpace(r):
			builder.WriteRune('_')
		default:
			builder.WriteRune('_')
		}
	}
	value := filenameUnderscoreRe.ReplaceAllString(builder.String(), "_")
	value = strings.Trim(value, " ._")
	if value == "" {
		return "download.bin"
	}
	return ensureSafeFilenameBase(value)
}

func encodeRFC5987(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	return encoded
}

func ensureSafeFilenameBase(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "download.bin"
	}
	ext := filepath.Ext(trimmed)
	base := strings.TrimSpace(strings.TrimSuffix(trimmed, ext))
	if base == "" {
		base = "download"
	}
	if _, reserved := windowsReservedNames[strings.ToLower(base)]; reserved {
		base += "_file"
	}
	base = strings.Trim(base, " ._")
	if base == "" {
		base = "download"
	}
	return base + ext
}

func sanitizeExtension(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(ext, ".")))
	if !extAllowedRe.MatchString(ext) {
		return "bin"
	}
	return ext
}

func shortHash(v string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(v)))
	return hex.EncodeToString(sum[:])[:6]
}

func truncateComponent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.Trim(safeTruncateUTF8(s, max), "_")
}

func truncateFilename(filename string, maxLen int) string {
	if len(filename) <= maxLen {
		return filename
	}
	return strings.Trim(safeTruncateUTF8(filename, maxLen), "_")
}

func safeTruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && maxBytes < len(s) && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

func extensionFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "video/mp4", "audio/mp4":
		return "mp4"
	case "audio/mpeg":
		return "mp3"
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func extensionFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(parsed.Path)), ".")
	return ext
}

func extensionFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image":
		return "jpg"
	case "audio":
		return "mp3"
	default:
		return "mp4"
	}
}

func normalizeContentType(current string, media []MediaItem) string {
	current = strings.ToLower(strings.TrimSpace(current))
	if current != "" && current != "post" {
		return current
	}
	types := map[string]struct{}{}
	for _, item := range media {
		itemType := strings.ToLower(strings.TrimSpace(item.Type))
		if itemType != "" {
			types[itemType] = struct{}{}
		}
	}
	if len(types) != 1 {
		return "post"
	}
	for itemType := range types {
		switch itemType {
		case "video", "audio", "image":
			return itemType
		}
	}
	return "post"
}

func primaryMediaItems(items []MediaItem) []MediaItem {
	ordered := make([]MediaItem, 0, len(items))
	appendType := func(target string) {
		for _, item := range items {
			if strings.EqualFold(strings.TrimSpace(item.Type), target) {
				ordered = append(ordered, item)
			}
		}
	}
	appendType("video")
	appendType("audio")
	appendType("image")
	for _, item := range items {
		itemType := strings.ToLower(strings.TrimSpace(item.Type))
		if itemType != "video" && itemType != "audio" && itemType != "image" {
			ordered = append(ordered, item)
		}
	}
	if len(ordered) == 0 {
		return items
	}
	return ordered
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func SourceOrigin(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func SanitizeStat(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func SumMediaSizes(items []MediaItem) int64 {
	var total int64
	for _, item := range items {
		total += item.FileSizeBytes
	}
	return total
}

func ContainerAndMIMEFromURL(rawURL string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(lower, ".m3u8"):
		return "m3u8", "application/vnd.apple.mpegurl"
	case strings.Contains(lower, ".mp4"):
		return "mp4", "video/mp4"
	case strings.Contains(lower, ".png"):
		return "png", "image/png"
	case strings.Contains(lower, ".webp"):
		return "webp", "image/webp"
	default:
		return "jpg", "image/jpeg"
	}
}

func ParseHumanInt(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, ",", ""), " ", ""))
	if s == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)([kmb]?)`)
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	switch strings.ToUpper(m[2]) {
	case "K":
		f *= 1_000
	case "M":
		f *= 1_000_000
	case "B":
		f *= 1_000_000_000
	}
	return int64(f)
}
