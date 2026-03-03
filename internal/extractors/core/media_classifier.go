package core

import (
	"strings"

	mediapkg "fetchmoona/pkg/media"
)

// MediaClassification contains normalized media detection output.
type MediaClassification struct {
	MediaType MediaType
	Mime      string
	Extension string
}

// ClassifyMedia determines media type using MIME first, then extension, then codec fallback.
func ClassifyMedia(mimeType, extension, vCodec, aCodec string) MediaClassification {
	mime := normalizeMimeType(mimeType)
	ext := normalizeExtension(extension)

	if mime != "" {
		if mimeExt := extensionFromMime(mime); mimeExt != "" {
			ext = mimeExt
		}
		if mediaType := mediaTypeFromMime(mime); mediaType != MediaTypeUnknown {
			return MediaClassification{MediaType: mediaType, Mime: mime, Extension: ext}
		}
	}

	if ext != "" {
		if mime == "" {
			mime = mimeFromExtension(ext)
		}
		if mediaType := mediaTypeFromExtension(ext); mediaType != MediaTypeUnknown {
			return MediaClassification{MediaType: mediaType, Mime: mime, Extension: ext}
		}
	}

	mediaType := mediaTypeFromCodecs(vCodec, aCodec)
	if mediaType != MediaTypeUnknown {
		if mime == "" {
			mime = fallbackMimeForMediaType(mediaType)
		}
		if ext == "" {
			ext = extensionFromMime(mime)
		}
	}

	return MediaClassification{MediaType: mediaType, Mime: mime, Extension: ext}
}

// AggregateMediaTypes derives one top-level type from multiple variant/media classifications.
func AggregateMediaTypes(types []MediaType) MediaType {
	hasVideo := false
	hasAudio := false
	hasImage := false

	for _, mt := range types {
		switch mt {
		case MediaTypeVideo:
			hasVideo = true
		case MediaTypeAudio:
			hasAudio = true
		case MediaTypeImage:
			hasImage = true
		}
	}

	if hasVideo {
		return MediaTypeVideo
	}
	if hasAudio {
		return MediaTypeAudio
	}
	if hasImage {
		return MediaTypeImage
	}

	return MediaTypeUnknown
}

func normalizeMimeType(mimeType string) string {
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	return mime
}

func normalizeExtension(extension string) string {
	ext := strings.ToLower(strings.TrimSpace(extension))
	ext = strings.TrimPrefix(ext, ".")
	return ext
}

func mediaTypeFromMime(mime string) MediaType {
	switch mediapkg.GetKindFromMime(mime) {
	case mediapkg.KindVideo:
		return MediaTypeVideo
	case mediapkg.KindAudio:
		return MediaTypeAudio
	case mediapkg.KindImage:
		return MediaTypeImage
	case mediapkg.KindPlaylist:
		return MediaTypeVideo
	default:
		return MediaTypeUnknown
	}
}

func mediaTypeFromExtension(ext string) MediaType {
	mime := mimeFromExtension(ext)
	if mime == "" {
		return MediaTypeUnknown
	}
	return mediaTypeFromMime(mime)
}

func mimeFromExtension(ext string) string {
	mime := mediapkg.GetMimeFromExtension(ext)
	if mime == "application/octet-stream" {
		return ""
	}
	return mime
}

func extensionFromMime(mime string) string {
	ext := normalizeExtension(GetExtensionFromMime(mime))
	if ext == "bin" {
		return ""
	}
	return ext
}

func mediaTypeFromCodecs(vCodec, aCodec string) MediaType {
	v := normalizeCodec(vCodec)
	a := normalizeCodec(aCodec)

	if v != "" {
		return MediaTypeVideo
	}
	if a != "" {
		return MediaTypeAudio
	}

	return MediaTypeUnknown
}

func normalizeCodec(codec string) string {
	c := strings.ToLower(strings.TrimSpace(codec))
	if c == "" || c == "none" {
		return ""
	}
	return c
}

func fallbackMimeForMediaType(mediaType MediaType) string {
	switch mediaType {
	case MediaTypeVideo:
		return "video/mp4"
	case MediaTypeAudio:
		return "audio/mp4"
	case MediaTypeImage:
		return "image/jpeg"
	default:
		return ""
	}
}
