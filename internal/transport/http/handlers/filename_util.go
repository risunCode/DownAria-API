package handlers

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

func resolveDownloadFilename(requestedFilename, upstreamContentDisposition, targetURL, platform, contentType string) string {
	format := detectFormat(contentType, targetURL)

	if requested := strings.TrimSpace(requestedFilename); requested != "" {
		return ensureFileExtension(requested, format)
	}

	if upstreamName := extractFilenameFromContentDisposition(upstreamContentDisposition); upstreamName != "" {
		if format == "" {
			return ensureFileExtension(upstreamName, "")
		}
		return ensureFileExtension(upstreamName, format)
	}

	base := "downaria_download"
	if p := strings.ToLower(strings.TrimSpace(platform)); p != "" {
		base = "downaria_" + p + "_download"
	}

	return ensureFileExtension(base, format)
}

func detectFormat(contentType, targetURL string) string {
	if mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType)); err == nil {
		switch strings.ToLower(strings.TrimSpace(mediaType)) {
		case "video/mp4":
			return "mp4"
		case "video/webm":
			return "webm"
		case "video/quicktime":
			return "mov"
		case "audio/mpeg":
			return "mp3"
		case "audio/mp4":
			return "m4a"
		case "audio/aac":
			return "aac"
		case "audio/ogg":
			return "ogg"
		case "image/jpeg":
			return "jpg"
		case "image/png":
			return "png"
		case "image/webp":
			return "webp"
		case "image/gif":
			return "gif"
		}
	}

	if targetURL == "" {
		return ""
	}
	base := path.Base(strings.TrimSpace(targetURL))
	if idx := strings.Index(base, "?"); idx >= 0 {
		base = base[:idx]
	}
	if ext := path.Ext(base); ext != "" {
		return strings.ToLower(strings.TrimPrefix(ext, "."))
	}
	return ""
}

func extractFilenameFromContentDisposition(contentDisposition string) string {
	v := strings.TrimSpace(contentDisposition)
	if v == "" {
		return ""
	}

	if mediaType, params, err := mime.ParseMediaType(v); err == nil {
		_ = mediaType
		if utf8Name, ok := params["filename*"]; ok && strings.TrimSpace(utf8Name) != "" {
			decoded := decodeRFC5987Filename(utf8Name)
			if strings.TrimSpace(decoded) != "" {
				return strings.TrimSpace(decoded)
			}
		}
		if name, ok := params["filename"]; ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}

	return ""
}

func decodeRFC5987Filename(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, "''", 2)
	if len(parts) == 2 {
		if decoded, err := urlQueryUnescape(parts[1]); err == nil {
			return decoded
		}
		return parts[1]
	}
	decoded, err := urlQueryUnescape(v)
	if err != nil {
		return v
	}
	return decoded
}

func urlQueryUnescape(value string) (string, error) {
	return url.QueryUnescape(value)
}
