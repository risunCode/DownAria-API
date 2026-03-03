package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
)

const SyndicationAPI = "https://cdn.syndication.twimg.com/tweet-result"

var tweetIDRegex = regexp.MustCompile(`/status/(\d+)`)

type TwitterExtractor struct {
	*core.BaseExtractor
}

func NewTwitterExtractor() *TwitterExtractor {
	return &TwitterExtractor{
		BaseExtractor: core.NewBaseExtractor(),
	}
}

func (e *TwitterExtractor) Match(urlStr string) bool {
	return e.MatchHost(urlStr, []string{"twitter.com", "x.com", "www.twitter.com", "www.x.com", "t.co"})
}

func (e *TwitterExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	normalizedURL, err := e.normalizeTweetURL(urlStr, opts.Ctx)
	if err != nil {
		return nil, err
	}

	// 1. Extract Tweet ID
	tweetID := e.extractTweetID(normalizedURL)
	if tweetID == "" {
		return nil, fmt.Errorf("invalid twitter URL: tweet ID not found")
	}

	// 2. Fetch via Syndication API (Public, no auth)
	apiURL := fmt.Sprintf("%s?id=%s&token=0", SyndicationAPI, tweetID)
	resp, err := e.MakeRequest("GET", apiURL, nil, opts, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := e.CheckStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}

	// 3. Parse Response
	var data struct {
		Text     string `json:"text"`
		FullText string `json:"full_text"`

		FavoriteCount     int64           `json:"favorite_count"`
		RetweetCount      int64           `json:"retweet_count"`
		ReplyCount        int64           `json:"reply_count"`
		ConversationCount int64           `json:"conversation_count"`
		ViewsCount        json.RawMessage `json:"views_count"`

		User struct {
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

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	mediaDetails := data.MediaDetails
	if len(mediaDetails) == 0 && len(data.MediaDetailsCamel) > 0 {
		mediaDetails = make([]struct {
			Type          string `json:"type"`
			MediaURLHTTPS string `json:"media_url_https"`
			VideoInfo     struct {
				Variants []struct {
					Bitrate int    `json:"bitrate"`
					URL     string `json:"url"`
				} `json:"variants"`
			} `json:"video_info"`
		}, len(data.MediaDetailsCamel))
		copy(mediaDetails, data.MediaDetailsCamel)
	}

	if len(mediaDetails) == 0 {
		return nil, fmt.Errorf("no media found in tweet")
	}

	// 4. Build Result using ResponseBuilder
	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("twitter").
		WithMediaType(core.MediaTypePost).
		WithAuthor(data.User.Name, data.User.ScreenName).
		WithContent(tweetID, pickFirstNonEmpty(data.FullText, data.Text), pickFirstNonEmpty(data.FullText, data.Text)).
		WithEngagement(
			sanitizeStat(parseViewsCount(data.ViewsCount)),
			sanitizeStat(data.FavoriteCount),
			sanitizeStat(e.resolveReplyCount(data.ReplyCount, data.ConversationCount)),
			sanitizeStat(data.RetweetCount),
		).
		WithAuthentication(opts.Cookie != "", opts.Source)

	for i, m := range mediaDetails {
		media := core.NewMedia(i, core.MediaTypeImage, m.MediaURLHTTPS)
		if m.Type == "video" || m.Type == "animated_gif" {
			media.Type = core.MediaTypeVideo

			// Collect all MP4 variants with bitrate info
			type variantInfo struct {
				Bitrate int
				URL     string
			}
			var variants []variantInfo

			for _, v := range m.VideoInfo.Variants {
				if strings.Contains(v.URL, ".mp4") {
					variants = append(variants, variantInfo{
						Bitrate: v.Bitrate,
						URL:     v.URL,
					})
				}
			}

			// Sort by bitrate descending (highest quality first)
			sort.Slice(variants, func(i, j int) bool {
				return variants[i].Bitrate > variants[j].Bitrate
			})

			// Create variant for each quality
			for _, v := range variants {
				quality := getQualityLabel(v.Bitrate)
				variant := core.NewVideoVariant(quality, v.URL).
					WithBitrate(v.Bitrate)

				filename := core.GenerateFilename(data.User.ScreenName, pickFirstNonEmpty(data.FullText, data.Text), tweetID, "mp4")
				variant = variant.WithFilename(filename)
				core.AddVariant(&media, variant)
			}
		} else {
			variant := core.NewImageVariant("Original", m.MediaURLHTTPS)
			filename := core.GenerateFilename(data.User.ScreenName, pickFirstNonEmpty(data.FullText, data.Text), tweetID, "jpg")
			variant = variant.WithFilename(filename)
			core.AddVariant(&media, variant)
		}
		builder.AddMedia(media)
	}

	return builder.Build(), nil
}

func (e *TwitterExtractor) extractTweetID(urlStr string) string {
	return util.ExtractFirstRegexGroup(urlStr, tweetIDRegex)
}

func (e *TwitterExtractor) normalizeTweetURL(urlStr string, ctx context.Context) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid twitter URL: %w", err)
	}

	host := strings.ToLower(u.Host)
	if host == "t.co" {
		resolved, err := e.resolveShortURL(urlStr, ctx)
		if err != nil {
			return "", err
		}
		urlStr = resolved
	}

	return urlStr, nil
}

func (e *TwitterExtractor) resolveShortURL(urlStr string, ctx context.Context) (string, error) {
	resp, err := e.MakeRequest(http.MethodGet, urlStr, nil, core.ExtractOptions{Ctx: ctx}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to resolve t.co URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.Request == nil || resp.Request.URL == nil {
		return "", fmt.Errorf("failed to resolve t.co URL: empty redirect target")
	}

	return resp.Request.URL.String(), nil
}

func (e *TwitterExtractor) resolveReplyCount(replyCount, conversationCount int64) int64 {
	if replyCount > conversationCount {
		return replyCount
	}
	return conversationCount
}

func sanitizeStat(value int64) int64 {
	return util.ClampNonNegativeInt64(value)
}

func parseViewsCount(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}

	var asInt int64
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return parseInt64(asString)
	}

	var asObject map[string]interface{}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if v, ok := asObject["count"]; ok {
			return parseUnknownInt(v)
		}
		if v, ok := asObject["value"]; ok {
			return parseUnknownInt(v)
		}
	}

	return 0
}

func parseUnknownInt(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case string:
		return parseInt64(v)
	case json.Number:
		n, err := v.Int64()
		if err == nil {
			return n
		}
	}
	return 0
}

func parseInt64(value string) int64 {
	return util.ParseInt64OrZero(value)
}

func pickFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// getQualityLabel converts bitrate (in bps) to user-friendly quality label
func getQualityLabel(bitrate int) string {
	bitrateMbps := bitrate / 1_000_000

	switch {
	case bitrateMbps >= 15:
		return "4K"
	case bitrateMbps >= 8:
		return "1440p"
	case bitrateMbps >= 5:
		return "1080p"
	case bitrateMbps >= 2:
		return "720p"
	case bitrateMbps >= 1:
		return "480p"
	case bitrateMbps >= 500/1000: // 500 kbps
		return "360p"
	default:
		return fmt.Sprintf("%d kbps", bitrate/1000)
	}
}
