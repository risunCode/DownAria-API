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
	case "mp3":
		return "audio/mpeg"
	case "m4a":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	case "opus":
		return "audio/opus"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "m3u8":
		return "application/vnd.apple.mpegurl"
	case "mpd":
		return "application/dash+xml"
	}
	return "application/octet-stream"
}

// GetKindFromMime determines the MediaKind from a MIME type.
func GetKindFromMime(mime string) MediaKind {
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
