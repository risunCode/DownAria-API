package twitter

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"downaria-api/internal/extract"
)

func decodeSyndication(reader io.Reader, statusID string) (*tweetData, error) {
	var payload struct {
		Typename          string `json:"__typename"`
		Text              string `json:"text"`
		FullText          string `json:"full_text"`
		FavoriteCount     int64  `json:"favorite_count"`
		RetweetCount      int64  `json:"retweet_count"`
		ReplyCount        int64  `json:"reply_count"`
		ConversationCount int64  `json:"conversation_count"`
		User              struct {
			Name       string `json:"name"`
			ScreenName string `json:"screen_name"`
		} `json:"user"`
		MediaDetails []struct {
			Type          string `json:"type"`
			MediaURLHTTPS string `json:"media_url_https"`
			VideoInfo     struct {
				Variants []struct {
					Bitrate int    `json:"bitrate"`
					URL     string `json:"url"`
				} `json:"variants"`
			} `json:"video_info"`
		} `json:"media_details"`
		MediaDetailsCamel []struct {
			Type          string `json:"type"`
			MediaURLHTTPS string `json:"media_url_https"`
			VideoInfo     struct {
				Variants []struct {
					Bitrate int    `json:"bitrate"`
					URL     string `json:"url"`
				} `json:"variants"`
			} `json:"video_info"`
		} `json:"mediaDetails"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "invalid twitter payload", err)
	}
	if strings.EqualFold(strings.TrimSpace(payload.Typename), "TweetTombstone") {
		return nil, extract.Wrap(extract.KindExtractionFailed, "no media found in tweet", nil)
	}
	mediaSource := payload.MediaDetails
	if len(mediaSource) == 0 && len(payload.MediaDetailsCamel) > 0 {
		mediaSource = make([]struct {
			Type          string `json:"type"`
			MediaURLHTTPS string `json:"media_url_https"`
			VideoInfo     struct {
				Variants []struct {
					Bitrate int    `json:"bitrate"`
					URL     string `json:"url"`
				} `json:"variants"`
			} `json:"video_info"`
		}, len(payload.MediaDetailsCamel))
		copy(mediaSource, payload.MediaDetailsCamel)
	}
	items := make([]extract.MediaItem, 0, len(mediaSource))
	for _, item := range mediaSource {
		mediaType := strings.ToLower(strings.TrimSpace(item.Type))
		switch mediaType {
		case "photo":
			url := strings.TrimSpace(item.MediaURLHTTPS)
			if url == "" {
				continue
			}
			items = append(items, extract.MediaItem{Index: len(items), Type: "image", ThumbnailURL: url, Sources: []extract.MediaSource{{Quality: "original", URL: url, MIMEType: "image/jpeg", Container: "jpg"}}})
		case "video", "animated_gif":
			sources := make([]extract.MediaSource, 0, len(item.VideoInfo.Variants))
			seen := map[string]struct{}{}
			for _, variant := range item.VideoInfo.Variants {
				variantURL := strings.TrimSpace(variant.URL)
				if variantURL == "" {
					continue
				}
				if _, ok := seen[variantURL]; ok {
					continue
				}
				seen[variantURL] = struct{}{}
				sources = append(sources, extract.MediaSource{Quality: qualityFromBitrate(variant.Bitrate), URL: variantURL, MIMEType: mimeTypeFromVariantURL(variantURL), Container: containerFromVariantURL(variantURL), HasVideo: true, HasAudio: strings.Contains(strings.ToLower(variantURL), ".mp4"), IsProgressive: strings.Contains(strings.ToLower(variantURL), ".mp4")})
			}
			sources = filterSources(sources)
			if len(sources) == 0 {
				continue
			}
			sort.Slice(sources, func(i, j int) bool { return twitterSourceLess(sources[i], sources[j]) })
			items = append(items, extract.MediaItem{Index: len(items), Type: "video", ThumbnailURL: strings.TrimSpace(item.MediaURLHTTPS), Sources: sources})
		}
	}
	if len(items) == 0 {
		return nil, extract.Wrap(extract.KindExtractionFailed, "no media found in tweet", nil)
	}
	comments := payload.ReplyCount
	if payload.ConversationCount > comments {
		comments = payload.ConversationCount
	}
	return &tweetData{ID: statusID, Text: extract.FirstNonEmpty(strings.TrimSpace(payload.FullText), strings.TrimSpace(payload.Text)), AuthorName: strings.TrimSpace(payload.User.Name), AuthorHandle: strings.TrimSpace(payload.User.ScreenName), Engagement: extract.Engagement{Likes: extract.SanitizeStat(payload.FavoriteCount), Comments: extract.SanitizeStat(comments), Shares: extract.SanitizeStat(payload.RetweetCount)}, Media: items}, nil
}

func qualityFromBitrate(bitrate int) string {
	if bitrate <= 0 {
		return "default"
	}
	bitrateMbps := bitrate / 1_000_000
	switch {
	case bitrateMbps >= 8:
		return "1440p"
	case bitrateMbps >= 5:
		return "1080p"
	case bitrateMbps >= 2:
		return "720p"
	case bitrateMbps >= 1:
		return "480p"
	default:
		return fmt.Sprintf("%dkbps", bitrate/1000)
	}
}

func mimeTypeFromVariantURL(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.Contains(lower, ".mp4"):
		return "video/mp4"
	default:
		return ""
	}
}

func containerFromVariantURL(rawURL string) string {
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, ".m3u8"):
		return "m3u8"
	case strings.Contains(lower, ".mp4"):
		return "mp4"
	default:
		return ""
	}
}

func filterSources(sources []extract.MediaSource) []extract.MediaSource {
	hasMP4 := false
	for _, source := range sources {
		if source.MIMEType == "video/mp4" {
			hasMP4 = true
			break
		}
	}
	if !hasMP4 {
		return sources
	}
	filtered := make([]extract.MediaSource, 0, len(sources))
	for _, source := range sources {
		if source.MIMEType != "application/vnd.apple.mpegurl" {
			filtered = append(filtered, source)
		}
	}
	if len(filtered) == 0 {
		return sources
	}
	return filtered
}

func twitterSourceLess(left, right extract.MediaSource) bool {
	if qualityRank(left.Quality) != qualityRank(right.Quality) {
		return qualityRank(left.Quality) > qualityRank(right.Quality)
	}
	if left.FileSizeBytes != right.FileSizeBytes {
		return left.FileSizeBytes > right.FileSizeBytes
	}
	return left.URL < right.URL
}

func qualityRank(value string) int {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "p")
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err == nil && n > 0 {
		return n
	}
	if strings.HasSuffix(value, "kbps") {
		if _, err := fmt.Sscanf(strings.TrimSuffix(value, "kbps"), "%d", &n); err == nil && n > 0 {
			return n / 10
		}
	}
	return 0
}
