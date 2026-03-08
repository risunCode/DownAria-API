package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
)

const (
	SyndicationAPI         = "https://cdn.syndication.twimg.com/tweet-result"
	twitterGraphQLEndpoint = "https://x.com/i/api/graphql"
	twitterGraphQLQueryID  = "xOhkmRac04YFZmOzU9PJHg"
	twitterBearerToken     = "Bearer AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"
)

var twitterGraphQLFeatures = map[string]bool{
	"creator_subscriptions_tweet_preview_api_enabled":                         true,
	"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
	"tweetypie_unmention_optimization_enabled":                                true,
	"responsive_web_edit_tweet_api_enabled":                                   true,
	"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
	"view_counts_everywhere_api_enabled":                                      true,
	"longform_notetweets_consumption_enabled":                                 true,
	"responsive_web_twitter_article_tweet_consumption_enabled":                false,
	"tweet_awards_web_tipping_enabled":                                        false,
	"responsive_web_home_pinned_timelines_enabled":                            true,
	"freedom_of_speech_not_reach_fetch_enabled":                               true,
	"standardized_nudges_misinfo":                                             true,
	"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
	"longform_notetweets_rich_text_read_enabled":                              true,
	"longform_notetweets_inline_media_enabled":                                true,
	"responsive_web_graphql_exclude_directive_enabled":                        true,
	"verified_phone_label_enabled":                                            false,
	"responsive_web_media_download_video_enabled":                             false,
	"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
	"responsive_web_graphql_timeline_navigation_enabled":                      true,
	"responsive_web_enhance_cards_enabled":                                    false,
}

var tweetIDRegex = regexp.MustCompile(`/status/(\d+)`)
var twitterResolutionRegex = regexp.MustCompile(`/(\d{2,5})x(\d{2,5})/`)

type TwitterExtractor struct {
	*core.BaseExtractor
}

type twitterVariant struct {
	Bitrate int
	URL     string
}

type twitterMediaDetail struct {
	Type          string
	MediaURLHTTPS string
	VideoVariants []twitterVariant
}

type twitterExtractData struct {
	TweetID           string
	Text              string
	AuthorName        string
	AuthorScreenName  string
	FavoriteCount     int64
	RetweetCount      int64
	ReplyCount        int64
	ConversationCount int64
	ViewsCount        int64
	MediaDetails      []twitterMediaDetail
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

	if syndication, err := e.fetchSyndicationExtractData(tweetID, opts); err == nil && len(syndication.MediaDetails) > 0 {
		return e.buildResult(urlStr, syndication, opts), nil
	}

	if strings.TrimSpace(opts.Cookie) != "" {
		graphqlData, err := e.fetchGraphQLExtractData(tweetID, opts)
		if err == nil && len(graphqlData.MediaDetails) > 0 {
			return e.buildResult(urlStr, graphqlData, opts), nil
		}
		if err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("no media found in tweet")
}

func (e *TwitterExtractor) fetchSyndicationExtractData(tweetID string, opts core.ExtractOptions) (*twitterExtractData, error) {
	apiURL := fmt.Sprintf("%s?id=%s&token=0", SyndicationAPI, tweetID)
	resp, err := e.MakeRequest(http.MethodGet, apiURL, nil, opts, map[string]string{
		"Accept":  "application/json",
		"Referer": "https://platform.twitter.com/",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := e.CheckStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var payload struct {
		Typename string `json:"__typename"`
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

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if strings.EqualFold(strings.TrimSpace(payload.Typename), "TweetTombstone") {
		return nil, fmt.Errorf("no media found in tweet")
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

	mediaDetails := make([]twitterMediaDetail, 0, len(mediaSource))
	for _, item := range mediaSource {
		mediaType := strings.ToLower(strings.TrimSpace(item.Type))
		entry := twitterMediaDetail{
			Type:          mediaType,
			MediaURLHTTPS: strings.TrimSpace(item.MediaURLHTTPS),
		}

		for _, variant := range item.VideoInfo.Variants {
			variantURL := strings.TrimSpace(variant.URL)
			if variantURL == "" {
				continue
			}
			if strings.Contains(variantURL, ".mp4") || strings.Contains(variantURL, ".m3u8") {
				entry.VideoVariants = append(entry.VideoVariants, twitterVariant{Bitrate: variant.Bitrate, URL: variantURL})
			}
		}

		if mediaType == "video" || mediaType == "animated_gif" {
			if len(entry.VideoVariants) == 0 {
				continue
			}
		}

		if mediaType == "photo" && entry.MediaURLHTTPS == "" {
			continue
		}

		if mediaType != "photo" && mediaType != "video" && mediaType != "animated_gif" {
			continue
		}

		mediaDetails = append(mediaDetails, entry)
	}

	if len(mediaDetails) == 0 {
		return nil, fmt.Errorf("no media found in tweet")
	}

	return &twitterExtractData{
		TweetID:           tweetID,
		Text:              pickFirstNonEmpty(payload.FullText, payload.Text),
		AuthorName:        payload.User.Name,
		AuthorScreenName:  payload.User.ScreenName,
		FavoriteCount:     sanitizeStat(payload.FavoriteCount),
		RetweetCount:      sanitizeStat(payload.RetweetCount),
		ReplyCount:        sanitizeStat(payload.ReplyCount),
		ConversationCount: sanitizeStat(payload.ConversationCount),
		ViewsCount:        sanitizeStat(parseViewsCount(payload.ViewsCount)),
		MediaDetails:      mediaDetails,
	}, nil
}

func (e *TwitterExtractor) fetchGraphQLExtractData(tweetID string, opts core.ExtractOptions) (*twitterExtractData, error) {
	ct0 := extractCt0Token(opts.Cookie)
	if ct0 == "" {
		return nil, fmt.Errorf("login required: missing ct0 token")
	}

	apiURL, err := buildGraphQLURL(tweetID)
	if err != nil {
		return nil, err
	}

	resp, err := e.MakeRequest(http.MethodGet, apiURL, nil, opts, map[string]string{
		"Accept":                    "*/*",
		"Authorization":             twitterBearerToken,
		"X-Csrf-Token":              ct0,
		"X-Twitter-Auth-Type":       "OAuth2Session",
		"X-Twitter-Active-User":     "yes",
		"X-Twitter-Client-Language": "en",
		"Origin":                    "https://x.com",
		"Referer":                   "https://x.com/",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return parseGraphQLPayload(payload, tweetID)
}

func (e *TwitterExtractor) buildResult(urlStr string, data *twitterExtractData, opts core.ExtractOptions) *core.ExtractResult {
	if data == nil {
		data = &twitterExtractData{}
	}

	tweetID := pickFirstNonEmpty(data.TweetID, e.extractTweetID(urlStr))
	text := strings.TrimSpace(data.Text)
	authorName := strings.TrimSpace(data.AuthorName)
	authorScreenName := strings.TrimSpace(data.AuthorScreenName)

	mediaType := core.MediaTypePost
	for _, media := range data.MediaDetails {
		if media.Type == "video" || media.Type == "animated_gif" {
			mediaType = core.MediaTypeVideo
			break
		}
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("twitter").
		WithMediaType(mediaType).
		WithAuthor(authorName, authorScreenName).
		WithContent(tweetID, text, text).
		WithEngagement(
			sanitizeStat(data.ViewsCount),
			sanitizeStat(data.FavoriteCount),
			sanitizeStat(e.resolveReplyCount(data.ReplyCount, data.ConversationCount)),
			sanitizeStat(data.RetweetCount),
		).
		WithAuthentication(opts.Cookie != "", opts.Source)

	filenameSeed := pickFirstNonEmpty(authorScreenName, authorName, "twitter")

	for idx, item := range data.MediaDetails {
		media := core.NewMedia(idx, core.MediaTypeImage, item.MediaURLHTTPS)
		if item.Type == "video" || item.Type == "animated_gif" {
			media.Type = core.MediaTypeVideo

			variants := make([]twitterVariant, 0, len(item.VideoVariants))
			seen := map[string]struct{}{}
			for _, variant := range item.VideoVariants {
				variantURL := strings.TrimSpace(variant.URL)
				if variantURL == "" {
					continue
				}
				if _, exists := seen[variantURL]; exists {
					continue
				}
				seen[variantURL] = struct{}{}
				variants = append(variants, twitterVariant{Bitrate: variant.Bitrate, URL: variantURL})
			}

			variants = filterHLSVariantsWhenProgressiveAudioExists(item.Type, variants)

			sort.Slice(variants, func(i, j int) bool {
				return variants[i].Bitrate > variants[j].Bitrate
			})

			for _, variantInfo := range variants {
				quality := getQualityLabel(variantInfo.Bitrate)
				variant := core.NewVideoVariant(quality, variantInfo.URL).WithBitrate(variantInfo.Bitrate)
				if qualityByURL, resolution := qualityFromVariantURL(variantInfo.URL); qualityByURL != "" {
					variant = variant.WithResolution(resolution)
					variant.Quality = qualityByURL
				}
				if isHLSVariantURL(variantInfo.URL) {
					variant = variant.
						WithFormat("m3u8").
						WithMime("application/vnd.apple.mpegurl").
						WithProxy(true)
				}
				filename := core.GenerateFilename(filenameSeed, text, "", "mp4")
				variant = variant.WithFilename(filename)
				core.AddVariant(&media, variant)
			}
		} else {
			variant := core.NewImageVariant("Original", item.MediaURLHTTPS)
			filename := core.GenerateFilename(filenameSeed, text, "", "jpg")
			variant = variant.WithFilename(filename)
			core.AddVariant(&media, variant)
		}
		builder.AddMedia(media)
	}

	return builder.Build()
}

func buildGraphQLURL(tweetID string) (string, error) {
	variables := map[string]interface{}{
		"focalTweetId":                           tweetID,
		"with_rux_injections":                    false,
		"includePromotedContent":                 true,
		"withCommunity":                          true,
		"withQuickPromoteEligibilityTweetFields": true,
		"withBirdwatchNotes":                     true,
		"withVoice":                              true,
		"withV2Timeline":                         true,
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return "", err
	}
	featuresJSON, err := json.Marshal(twitterGraphQLFeatures)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/TweetDetail?variables=%s&features=%s",
		twitterGraphQLEndpoint,
		twitterGraphQLQueryID,
		url.QueryEscape(string(variablesJSON)),
		url.QueryEscape(string(featuresJSON)),
	), nil
}

func extractCt0Token(cookie string) string {
	parts := strings.Split(cookie, ";")
	for _, part := range parts {
		pair := strings.TrimSpace(part)
		if strings.HasPrefix(pair, "ct0=") {
			return strings.TrimSpace(strings.TrimPrefix(pair, "ct0="))
		}
	}
	return ""
}

func parseGraphQLPayload(payload map[string]interface{}, targetTweetID string) (*twitterExtractData, error) {
	var candidates []*twitterExtractData
	collectGraphQLCandidates(payload, &candidates)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no media found in tweet")
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if isBetterGraphQLCandidate(candidate, best, targetTweetID) {
			best = candidate
		}
	}

	if best == nil || len(best.MediaDetails) == 0 {
		return nil, fmt.Errorf("no media found in tweet")
	}

	if best.TweetID == "" {
		best.TweetID = targetTweetID
	}

	return best, nil
}

func collectGraphQLCandidates(node interface{}, candidates *[]*twitterExtractData) {
	switch value := node.(type) {
	case map[string]interface{}:
		if candidate := graphQLCandidateFromNode(value); candidate != nil {
			*candidates = append(*candidates, candidate)
		}
		for _, nested := range value {
			collectGraphQLCandidates(nested, candidates)
		}
	case []interface{}:
		for _, nested := range value {
			collectGraphQLCandidates(nested, candidates)
		}
	}
}

func graphQLCandidateFromNode(node map[string]interface{}) *twitterExtractData {
	legacy := asMap(node["legacy"])
	if legacy == nil {
		return nil
	}

	mediaDetails := parseGraphQLMedia(legacy)
	if len(mediaDetails) == 0 {
		return nil
	}

	authorName, authorScreenName := parseGraphQLAuthor(node)

	return &twitterExtractData{
		TweetID:           strings.TrimSpace(asString(node["rest_id"])),
		Text:              pickFirstNonEmpty(asString(legacy["full_text"]), asString(legacy["text"])),
		AuthorName:        authorName,
		AuthorScreenName:  authorScreenName,
		FavoriteCount:     sanitizeStat(asInt64(legacy["favorite_count"])),
		RetweetCount:      sanitizeStat(asInt64(legacy["retweet_count"])),
		ReplyCount:        sanitizeStat(asInt64(legacy["reply_count"])),
		ConversationCount: sanitizeStat(asInt64(legacy["conversation_count"])),
		ViewsCount:        sanitizeStat(parseViewsUnknown(node["views"])),
		MediaDetails:      mediaDetails,
	}
}

func parseGraphQLMedia(legacy map[string]interface{}) []twitterMediaDetail {
	mediaItems := asSlice(asMap(legacy["extended_entities"])["media"])
	if len(mediaItems) == 0 {
		mediaItems = asSlice(asMap(legacy["entities"])["media"])
	}

	mediaDetails := make([]twitterMediaDetail, 0, len(mediaItems))
	for _, mediaNode := range mediaItems {
		mediaMap := asMap(mediaNode)
		if mediaMap == nil {
			continue
		}

		mediaType := strings.ToLower(strings.TrimSpace(asString(mediaMap["type"])))
		if mediaType == "" {
			continue
		}

		detail := twitterMediaDetail{
			Type:          mediaType,
			MediaURLHTTPS: strings.TrimSpace(asString(mediaMap["media_url_https"])),
		}

		videoInfo := asMap(mediaMap["video_info"])
		variantsRaw := asSlice(videoInfo["variants"])
		seen := map[string]struct{}{}
		for _, variantNode := range variantsRaw {
			variantMap := asMap(variantNode)
			if variantMap == nil {
				continue
			}
			variantURL := strings.TrimSpace(asString(variantMap["url"]))
			if variantURL == "" {
				continue
			}
			if _, exists := seen[variantURL]; exists {
				continue
			}
			if strings.Contains(variantURL, ".mp4") || strings.Contains(variantURL, ".m3u8") {
				seen[variantURL] = struct{}{}
				detail.VideoVariants = append(detail.VideoVariants, twitterVariant{
					Bitrate: int(asInt64(variantMap["bitrate"])),
					URL:     variantURL,
				})
			}
		}

		if mediaType == "video" || mediaType == "animated_gif" {
			if len(detail.VideoVariants) == 0 {
				continue
			}
		}

		if mediaType == "photo" && detail.MediaURLHTTPS == "" {
			continue
		}

		if mediaType != "photo" && mediaType != "video" && mediaType != "animated_gif" {
			continue
		}

		mediaDetails = append(mediaDetails, detail)
	}

	return mediaDetails
}

func parseGraphQLAuthor(node map[string]interface{}) (name string, screenName string) {
	coreNode := asMap(node["core"])
	userResults := asMap(coreNode["user_results"])
	result := asMap(userResults["result"])
	legacy := asMap(result["legacy"])
	if legacy == nil {
		legacy = asMap(asMap(result["result"])["legacy"])
	}
	if legacy == nil {
		return "", ""
	}

	return strings.TrimSpace(asString(legacy["name"])), strings.TrimSpace(asString(legacy["screen_name"]))
}

func isBetterGraphQLCandidate(candidate, current *twitterExtractData, targetTweetID string) bool {
	if current == nil {
		return true
	}

	candidateMatch := strings.TrimSpace(candidate.TweetID) == strings.TrimSpace(targetTweetID)
	currentMatch := strings.TrimSpace(current.TweetID) == strings.TrimSpace(targetTweetID)
	if candidateMatch != currentMatch {
		return candidateMatch
	}

	if len(candidate.MediaDetails) != len(current.MediaDetails) {
		return len(candidate.MediaDetails) > len(current.MediaDetails)
	}

	return candidate.ViewsCount > current.ViewsCount
}

func parseViewsUnknown(value interface{}) int64 {
	if value == nil {
		return 0
	}
	if direct := parseUnknownInt(value); direct > 0 {
		return direct
	}
	valueMap := asMap(value)
	if valueMap == nil {
		return 0
	}
	if count := parseUnknownInt(valueMap["count"]); count > 0 {
		return count
	}
	if value := parseUnknownInt(valueMap["value"]); value > 0 {
		return value
	}
	return 0
}

func asMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if out, ok := value.(map[string]interface{}); ok {
		return out
	}
	return nil
}

func asSlice(value interface{}) []interface{} {
	if value == nil {
		return nil
	}
	if out, ok := value.([]interface{}); ok {
		return out
	}
	return nil
}

func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	default:
		return ""
	}
}

func asInt64(value interface{}) int64 {
	if value == nil {
		return 0
	}
	return parseUnknownInt(value)
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

func isHLSVariantURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(strings.ToLower(rawURL), ".m3u8")
	}
	return strings.Contains(strings.ToLower(parsedURL.Path), ".m3u8")
}

func filterHLSVariantsWhenProgressiveAudioExists(mediaType string, variants []twitterVariant) []twitterVariant {
	if len(variants) == 0 || strings.TrimSpace(mediaType) != "video" {
		return variants
	}

	hasProgressiveWithAudio := false
	for _, variant := range variants {
		if isHLSVariantURL(variant.URL) {
			continue
		}
		if strings.Contains(strings.ToLower(variant.URL), ".mp4") && variant.Bitrate > 0 {
			hasProgressiveWithAudio = true
			break
		}
	}

	if !hasProgressiveWithAudio {
		return variants
	}

	filtered := make([]twitterVariant, 0, len(variants))
	for _, variant := range variants {
		if isHLSVariantURL(variant.URL) {
			continue
		}
		filtered = append(filtered, variant)
	}

	if len(filtered) == 0 {
		return variants
	}

	return filtered
}

func qualityFromVariantURL(rawURL string) (quality string, resolution string) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", ""
	}

	parsedURL, err := url.Parse(trimmed)
	pathValue := trimmed
	if err == nil {
		pathValue = parsedURL.Path
	}

	matches := twitterResolutionRegex.FindStringSubmatch(pathValue)
	if len(matches) != 3 {
		return "", ""
	}

	width, errW := strconv.Atoi(matches[1])
	height, errH := strconv.Atoi(matches[2])
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return "", ""
	}

	resolution = fmt.Sprintf("%dx%d", width, height)
	quality = fmt.Sprintf("%dp", minInt(width, height))
	return quality, resolution
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
