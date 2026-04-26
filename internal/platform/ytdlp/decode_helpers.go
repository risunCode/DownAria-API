package ytdlp

import (
	"net/url"
	"strconv"
	"strings"

	"downaria-api/internal/extract"
)

func qualityFromVariantURL(rawURL string) string {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	for _, token := range []string{"2160p", "1440p", "1080p", "720p", "480p", "360p", "240p", "144p"} {
		if strings.Contains(lower, token) {
			return token
		}
	}
	return ""
}

func inferredHeight(format Format) int {
	if format.Height > 0 {
		return format.Height
	}
	quality := qualityFromVariantURL(format.URL)
	quality = strings.TrimSuffix(quality, "p")
	height, _ := strconv.Atoi(quality)
	return height
}

func inferredWidth(format Format) int {
	if format.Width > 0 {
		return format.Width
	}
	switch inferredHeight(format) {
	case 2160:
		return 3840
	case 1440:
		return 2560
	case 1080:
		return 1920
	case 720:
		return 1280
	case 480:
		return 854
	case 360:
		return 640
	case 240:
		return 426
	case 144:
		return 256
	default:
		return 0
	}
}

func variantMIMEType(format Format) string {
	if mimeType := strings.TrimSpace(format.MIMEType); mimeType != "" {
		return mimeType
	}
	return extToMIME(strings.TrimSpace(format.Ext))
}

func formatDuration(dump *Dump, format Format) float64 {
	if format.Duration > 0 {
		return format.Duration
	}
	if dump != nil && dump.Duration > 0 {
		return dump.Duration
	}
	return 0
}

func extToMIME(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case "mp4":
		return "video/mp4"
	case "m4a":
		return "audio/mp4"
	case "webm":
		return "video/webm"
	case "mp3":
		return "audio/mpeg"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func isAudioOnly(format Format) bool {
	video := strings.TrimSpace(format.VCodec)
	audio := strings.TrimSpace(format.ACodec)
	return strings.EqualFold(video, "none") && audio != "" && !strings.EqualFold(audio, "none")
}

func hasAudio(format Format) bool {
	codec := strings.TrimSpace(format.ACodec)
	return codec != "" && !strings.EqualFold(codec, "none")
}

func hasVideo(format Format) bool {
	codec := strings.TrimSpace(format.VCodec)
	return codec != "" && !strings.EqualFold(codec, "none")
}

func formatSize(format Format) int64 {
	if format.FileSize > 0 {
		return format.FileSize
	}
	if format.FileSizeApprox > 0 {
		return format.FileSizeApprox
	}
	return 0
}

func sourceLess(left, right extract.MediaSource) bool {
	if left.Height != right.Height {
		return left.Height > right.Height
	}
	if left.Width != right.Width {
		return left.Width > right.Width
	}
	if left.FileSizeBytes != right.FileSizeBytes {
		return left.FileSizeBytes > right.FileSizeBytes
	}
	if left.IsProgressive != right.IsProgressive {
		return left.IsProgressive
	}
	if left.FormatID != right.FormatID {
		return left.FormatID < right.FormatID
	}
	return left.URL < right.URL
}

func sumSourceSizes(items []extract.MediaSource) int64 {
	var max int64
	for _, item := range items {
		if item.FileSizeBytes > max {
			max = item.FileSizeBytes
		}
	}
	return max
}

func aggregateDumpSize(dump *Dump) int64 {
	if dump == nil {
		return 0
	}
	var max int64
	for _, format := range dump.Formats {
		if size := formatSize(format); size > max {
			max = size
		}
	}
	for _, entry := range dump.Entries {
		if size := aggregateDumpSize(&entry); size > max {
			max = size
		}
	}
	return max
}

func parseHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
