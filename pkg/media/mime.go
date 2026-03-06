package media

import "strings"

// MediaKind represents the type of media
type MediaKind string

const (
	KindVideo    MediaKind = "video"
	KindAudio    MediaKind = "audio"
	KindImage    MediaKind = "image"
	KindPlaylist MediaKind = "playlist"
	KindUnknown  MediaKind = "unknown"
)

// GetMimeFromExtension returns the MIME type for a given file extension.
func GetMimeFromExtension(ext string) string {
	ext = strings.TrimPrefix(ext, ".")
	ext = strings.ToLower(ext)
	switch ext {
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mov":
		return "video/quicktime"
	case "m4v":
		return "video/x-m4v"
	case "mpeg", "mpg":
		return "video/mpeg"
	case "avi":
		return "video/x-msvideo"
	case "mkv":
		return "video/x-matroska"
	case "ts":
		return "video/mp2t"
	case "mp3":
		return "audio/mpeg"
	case "m4a":
		return "audio/mp4"
	case "aac":
		return "audio/aac"
	case "ogg":
		return "audio/ogg"
	case "opus":
		return "audio/opus"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "svg":
		return "image/svg+xml"
	case "m3u8":
		return "application/vnd.apple.mpegurl"
	case "m3u":
		return "application/x-mpegurl"
	case "mpd":
		return "application/dash+xml"
	}
	return "application/octet-stream"
}

// GetExtensionFromMime returns file extension for a given MIME type.
func GetExtensionFromMime(mime string) string {
	normalized := normalizeMime(mime)

	switch normalized {
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "video/x-m4v":
		return "m4v"
	case "video/mpeg":
		return "mpeg"
	case "video/x-msvideo":
		return "avi"
	case "video/x-matroska":
		return "mkv"
	case "video/mp2t":
		return "ts"
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4", "audio/x-m4a":
		return "m4a"
	case "audio/aac", "audio/x-aac":
		return "aac"
	case "audio/ogg":
		return "ogg"
	case "audio/opus":
		return "opus"
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav"
	case "audio/webm":
		return "webm"
	case "audio/flac", "audio/x-flac":
		return "flac"
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
	case "application/vnd.apple.mpegurl", "application/x-mpegurl":
		return "m3u8"
	case "application/dash+xml":
		return "mpd"
	default:
		return "bin"
	}
}

// GetKindFromMime determines the MediaKind from a MIME type.
func GetKindFromMime(mime string) MediaKind {
	mime = normalizeMime(mime)

	if strings.HasPrefix(mime, "video/") {
		return KindVideo
	}
	if strings.HasPrefix(mime, "audio/") {
		return KindAudio
	}
	if strings.HasPrefix(mime, "image/") {
		return KindImage
	}
	if strings.Contains(mime, "mpegurl") || strings.Contains(mime, "dash") {
		return KindPlaylist
	}
	return KindUnknown
}

func normalizeMime(mime string) string {
	v := strings.ToLower(strings.TrimSpace(mime))
	if idx := strings.Index(v, ";"); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	return v
}
