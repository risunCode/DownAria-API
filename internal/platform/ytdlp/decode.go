package ytdlp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"downaria-api/internal/extract"
)

type Dump struct {
	ID               string   `json:"id"`
	WebpageURL       string   `json:"webpage_url"`
	WebpageURLDomain string   `json:"webpage_url_domain"`
	OriginalURL      string   `json:"original_url"`
	Extractor        string   `json:"extractor"`
	ExtractorKey     string   `json:"extractor_key"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Uploader         string   `json:"uploader"`
	UploaderID       string   `json:"uploader_id"`
	Thumbnail        string   `json:"thumbnail"`
	URL              string   `json:"url"`
	Ext              string   `json:"ext"`
	Duration         float64  `json:"duration"`
	ViewCount        int64    `json:"view_count"`
	LikeCount        int64    `json:"like_count"`
	CommentCount     int64    `json:"comment_count"`
	RepostCount      int64    `json:"repost_count"`
	ShareCount       int64    `json:"share_count"`
	BookmarkCount    int64    `json:"bookmark_count"`
	Formats          []Format `json:"formats"`
	Entries          []Dump   `json:"entries"`
}

type Format struct {
	FormatID       string  `json:"format_id"`
	URL            string  `json:"url"`
	Ext            string  `json:"ext"`
	FormatNote     string  `json:"format_note"`
	Resolution     string  `json:"resolution"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	MIMEType       string  `json:"mime_type"`
	Protocol       string  `json:"protocol"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Duration       float64 `json:"duration"`
	FileSize       int64   `json:"filesize"`
	FileSizeApprox int64   `json:"filesize_approx"`
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func DecodeDump(data []byte) (*Dump, error) {
	var dump Dump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, err
	}
	if strings.TrimSpace(dump.WebpageURL) == "" && strings.TrimSpace(dump.URL) == "" && len(dump.Formats) == 0 && len(dump.Entries) == 0 {
		return nil, fmt.Errorf("yt-dlp output has no media")
	}
	return &dump, nil
}

func MapResult(rawURL string, dump *Dump) (*extract.Result, error) {
	if dump == nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "yt-dlp output is empty", nil)
	}
	platform := detectPlatform(rawURL, dump)
	media := collectMediaItems(dump, platform)
	media = normalizeDirectProgressiveVideos(media)
	if len(media) == 0 {
		return nil, extract.Wrap(extract.KindExtractionFailed, "yt-dlp output has no variants", nil)
	}
	for i := range media {
		media[i].Index = i
	}
	return extract.NewResultBuilder(extract.FirstNonEmpty(strings.TrimSpace(dump.WebpageURL), rawURL), platform, "generic").
		Title(strings.TrimSpace(dump.Title)).
		Author(strings.TrimSpace(dump.Uploader), strings.TrimSpace(dump.UploaderID)).
		Engagement(dump.ViewCount, dump.LikeCount, dump.CommentCount, maxInt64(dump.RepostCount, dump.ShareCount), dump.BookmarkCount).
		Media(media).
		Build(), nil
}

func normalizeDirectProgressiveVideos(media []extract.MediaItem) []extract.MediaItem {
	for i := range media {
		if media[i].Type != "video" {
			continue
		}
		for j := range media[i].Sources {
			source := &media[i].Sources[j]
			if !isLikelyDirectProgressiveVideo(*source, media[i].Type) {
				continue
			}
			source.HasVideo = true
			if codec := strings.TrimSpace(source.AudioCodec); codec != "" && !strings.EqualFold(codec, "none") {
				source.HasAudio = true
			} else if shouldAssumeDirectProgressiveAudio(*source) {
				source.HasAudio = true
			}
			source.IsProgressive = source.HasVideo && source.HasAudio
		}
	}
	return media
}

func shouldAssumeDirectProgressiveAudio(source extract.MediaSource) bool {
	value := strings.ToLower(strings.TrimSpace(extract.FirstNonEmpty(source.URL, source.Referer, source.Origin)))
	return !strings.Contains(value, "googlevideo.com") && !strings.Contains(value, "youtube.com") && !strings.Contains(value, "youtu.be")
}

func isLikelyDirectProgressiveVideo(source extract.MediaSource, mediaType string) bool {
	if source.IsProgressive || source.HasAudio {
		return false
	}
	if !source.HasVideo && strings.TrimSpace(source.VideoCodec) == "" && strings.TrimSpace(mediaType) != "video" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(source.Container)) != "mp4" {
		return false
	}
	protocol := strings.ToLower(strings.TrimSpace(source.Protocol))
	if protocol != "" && protocol != "https" && protocol != "http" {
		return false
	}
	if strings.TrimSpace(source.URL) == "" {
		return false
	}
	lowerURL := strings.ToLower(strings.TrimSpace(source.URL))
	if strings.Contains(lowerURL, ".m3u8") || strings.Contains(lowerURL, ".mpd") {
		return false
	}
	if strings.TrimSpace(source.Referer) == "" && strings.TrimSpace(source.Origin) == "" {
		return false
	}
	if source.Height <= 0 && qualityFromVariantURL(source.URL) == "" && strings.TrimSpace(source.Quality) == "" {
		return false
	}
	return true
}

func collectMediaItems(dump *Dump, platform string) []extract.MediaItem {
	if dump == nil {
		return nil
	}
	if len(dump.Entries) > 0 {
		items := make([]extract.MediaItem, 0, len(dump.Entries))
		for _, entry := range dump.Entries {
			items = append(items, collectMediaItems(&entry, platform)...)
		}
		return dedupeMediaItems(items)
	}
	videoSources, audioSources, imageSources := []extract.MediaSource{}, []extract.MediaSource{}, []extract.MediaSource{}
	for _, format := range dump.Formats {
		variantURL := strings.TrimSpace(format.URL)
		if variantURL == "" || strings.Contains(variantURL, "storyboard") || strings.Contains(variantURL, "/sb/") {
			continue
		}
		hasVideo := hasVideo(format)
		hasAudio := hasAudio(format)
		// YouTube muxed format heuristic (format 18, 22, etc)
		isYoutubeMuxed := platform == "youtube" && (format.FormatID == "18" || format.FormatID == "22")
		if isYoutubeMuxed {
			hasVideo = true
			hasAudio = true
		}
		source := extract.MediaSource{FormatID: strings.TrimSpace(format.FormatID), Quality: variantLabel(format), URL: variantURL, Referer: sourceReferer(dump), Origin: extract.SourceOrigin(sourceReferer(dump)), MIMEType: variantMIMEType(format), Protocol: strings.TrimSpace(format.Protocol), Container: strings.ToLower(strings.TrimSpace(format.Ext)), VideoCodec: strings.TrimSpace(format.VCodec), AudioCodec: strings.TrimSpace(format.ACodec), Width: inferredWidth(format), Height: inferredHeight(format), DurationSeconds: formatDuration(dump, format), FileSizeBytes: formatSize(format), HasAudio: hasAudio, HasVideo: hasVideo, IsProgressive: hasVideo && hasAudio}
		if strings.HasPrefix(source.MIMEType, "image/") {
			imageSources = append(imageSources, source)
			continue
		}
		if isAudioOnly(format) {
			audioSources = append(audioSources, source)
			continue
		}
		if !strings.HasPrefix(source.MIMEType, "video/") && source.MIMEType != "application/vnd.apple.mpegurl" {
			continue
		}
		videoSources = append(videoSources, source)
	}
	if strings.TrimSpace(dump.URL) != "" {
		fallback := extract.MediaSource{Quality: strings.ToUpper(strings.TrimSpace(dump.Ext)), URL: strings.TrimSpace(dump.URL), Referer: sourceReferer(dump), Origin: extract.SourceOrigin(sourceReferer(dump)), MIMEType: extToMIME(strings.TrimSpace(dump.Ext)), Container: strings.ToLower(strings.TrimSpace(dump.Ext)), DurationSeconds: dump.Duration, FileSizeBytes: aggregateDumpSize(dump), HasAudio: strings.HasPrefix(extToMIME(strings.TrimSpace(dump.Ext)), "audio/"), HasVideo: strings.HasPrefix(extToMIME(strings.TrimSpace(dump.Ext)), "video/")}
		fallback.IsProgressive = fallback.HasVideo && fallback.HasAudio
		switch {
		case strings.HasPrefix(fallback.MIMEType, "video/") || fallback.MIMEType == "application/vnd.apple.mpegurl":
			if len(videoSources) == 0 {
				videoSources = append(videoSources, fallback)
			}
		case strings.HasPrefix(fallback.MIMEType, "audio/"):
			if len(audioSources) == 0 {
				audioSources = append(audioSources, fallback)
			}
		case strings.HasPrefix(fallback.MIMEType, "image/"):
			if len(imageSources) == 0 {
				imageSources = append(imageSources, fallback)
			}
		}
	}
	videoSources = dedupeSources(videoSources)
	audioSources = dedupeAudioSources(audioSources)
	imageSources = dedupeSources(imageSources)
	sort.Slice(videoSources, func(i, j int) bool { return sourceLess(videoSources[i], videoSources[j]) })
	sort.Slice(audioSources, func(i, j int) bool { return sourceLess(audioSources[i], audioSources[j]) })
	sort.Slice(imageSources, func(i, j int) bool { return sourceLess(imageSources[i], imageSources[j]) })
	media := make([]extract.MediaItem, 0, 2)
	if len(imageSources) > 0 {
		preview := extract.FirstNonEmpty(strings.TrimSpace(dump.Thumbnail), imageSources[0].URL)
		media = append(media, extract.MediaItem{Type: "image", ThumbnailURL: preview, FileSizeBytes: sumSourceSizes(imageSources), Sources: imageSources})
	}
	if len(videoSources) > 0 {
		media = append(media, extract.MediaItem{Type: "video", ThumbnailURL: strings.TrimSpace(dump.Thumbnail), FileSizeBytes: sumSourceSizes(videoSources), Sources: videoSources})
	}
	if len(audioSources) > 0 {
		media = append(media, extract.MediaItem{Type: "audio", ThumbnailURL: strings.TrimSpace(dump.Thumbnail), FileSizeBytes: sumSourceSizes(audioSources), Sources: audioSources})
	}
	return media
}

func detectPlatform(rawURL string, dump *Dump) string {
	for _, candidate := range []string{domainPlatform(strings.TrimSpace(dump.WebpageURLDomain)), domainPlatform(parseHost(extract.FirstNonEmpty(strings.TrimSpace(dump.WebpageURL), strings.TrimSpace(dump.OriginalURL), rawURL))), extractorPlatform(strings.TrimSpace(dump.ExtractorKey)), extractorPlatform(strings.TrimSpace(dump.Extractor))} {
		if candidate != "" {
			return candidate
		}
	}
	return "generic"
}

func extractorPlatform(value string) string {
	clean := sanitizePlatformToken(value)
	if clean == "" || clean == "generic" {
		return ""
	}
	if strings.HasPrefix(clean, "youtube") || clean == "youtu" {
		return "youtube"
	}
	return clean
}

func domainPlatform(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return ""
	}
	parts := strings.Split(host, ".")
	if len(parts) == 1 {
		token := sanitizePlatformToken(parts[0])
		if token == "youtu" {
			return "youtube"
		}
		return token
	}
	index := len(parts) - 2
	if len(parts) >= 3 && isSecondLevelSuffix(parts[len(parts)-2]) && len(parts[len(parts)-1]) == 2 {
		index = len(parts) - 3
	}
	if index < 0 || index >= len(parts) {
		return ""
	}
	token := sanitizePlatformToken(parts[index])
	if token == "youtu" {
		return "youtube"
	}
	return token
}

func sanitizePlatformToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = nonAlphaNum.ReplaceAllString(value, "")
	return value
}

func isSecondLevelSuffix(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "co", "com", "org", "net", "gov", "ac":
		return true
	default:
		return false
	}
}

func dedupeMediaItems(items []extract.MediaItem) []extract.MediaItem {
	seen := map[string]struct{}{}
	out := make([]extract.MediaItem, 0, len(items))
	for _, item := range items {
		key := item.Type + ":"
		if len(item.Sources) > 0 {
			key += item.Sources[0].URL
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeSources(items []extract.MediaSource) []extract.MediaSource {
	seen := map[string]struct{}{}
	out := make([]extract.MediaSource, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item.URL]; ok {
			continue
		}
		seen[item.URL] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeAudioSources(items []extract.MediaSource) []extract.MediaSource {
	seen := map[string]extract.MediaSource{}
	for _, item := range items {
		key := audioSourceKey(item)
		if key == "" {
			continue
		}
		current, ok := seen[key]
		if !ok || audioSourceScore(item) > audioSourceScore(current) {
			seen[key] = item
		}
	}
	if len(seen) == 0 {
		return items
	}
	out := make([]extract.MediaSource, 0, len(items))
	for _, item := range items {
		key := audioSourceKey(item)
		if key == "" {
			out = append(out, item)
			continue
		}
		if seen[key].URL == item.URL {
			out = append(out, item)
		}
	}
	return out
}

func audioSourceKey(item extract.MediaSource) string {
	key := strings.ToLower(strings.TrimSpace(item.Container))
	if key != "" {
		return key
	}
	key = strings.ToLower(strings.TrimSpace(item.MIMEType))
	if key != "" {
		return key
	}
	return strings.ToLower(strings.TrimSpace(item.URL))
}

func audioSourceScore(item extract.MediaSource) int64 {
	score := item.FileSizeBytes
	if item.HasAudio {
		score += 1_000_000_000
	}
	if strings.Contains(strings.ToLower(item.MIMEType), "audio/") {
		score += 10_000_000
	}
	return score
}

func variantLabel(format Format) string {
	if format.Height > 0 {
		return fmt.Sprintf("%dp", format.Height)
	}
	if quality := qualityFromVariantURL(format.URL); quality != "" {
		return quality
	}
	if note := strings.TrimSpace(format.FormatNote); note != "" {
		return note
	}
	if resolution := strings.TrimSpace(format.Resolution); resolution != "" && resolution != "audio only" {
		return resolution
	}
	if id := strings.TrimSpace(format.FormatID); id != "" {
		return id
	}
	if ext := strings.TrimSpace(format.Ext); ext != "" {
		return strings.ToUpper(ext)
	}
	return "default"
}
