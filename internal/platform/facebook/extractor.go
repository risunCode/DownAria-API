package facebook

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/outbound"
	"downaria-api/internal/platform/probe"
)

var (
	fbNumericRe               = regexp.MustCompile(`^\d+$`)
	fbQualityHeightPattern    = regexp.MustCompile(`(\d{3,4})p`)
	fbTitleViewsPattern       = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?\s*[kmb]?)\s*views\b`)
	fbTitleLikesPattern       = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?\s*[kmb]?)\s*(?:reactions?|likes?)\b`)
	fbTitleStatsPrefixRegex   = regexp.MustCompile(`(?i)^\s*(?:[0-9]+(?:[.,][0-9]+)?\s*[kmb]?\s*(?:views?|reactions?|likes?)\s*(?:·\s*)?)+\|\s*`)
	fbTitleSeparatorRegex     = regexp.MustCompile(`\s*([|·])\s*`)
	fbInlineThumbnailPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"preferred_thumbnail":\{"uri":"(https:[^"]+)"`),
		regexp.MustCompile(`"(?:previewImage|story_thumbnail|poster_image)":\{"uri":"(https:[^"]+)"`),
		regexp.MustCompile(`"thumbnailImage":\{"uri":"(https:[^"]+)"`),
	}
	fbAuthorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"name":"([^"]+)","enable_reels_tab_deeplink":true`),
		regexp.MustCompile(`"owning_profile":\{"__typename":"(?:User|Page)","name":"([^"]+)"`),
		regexp.MustCompile(`"owner":\{"__typename":"(?:User|Page)"[^}]*"name":"([^"]+)"`),
	}
	fbStoryAuthorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"story_bucket_owner":\{"__typename":"(?:User|Page)"[^}]*"name":"([^"]+)"`),
		regexp.MustCompile(`"story_owner":\{[^}]*"name":"([^"]+)"`),
	}
	fbCreatedAtNumericPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"creation_time":(\d{10,13})`),
		regexp.MustCompile(`"created_time":(\d{10,13})`),
		regexp.MustCompile(`"publish_time":(\d{10,13})`),
		regexp.MustCompile(`"creation_time":"(\d{10,13})"`),
		regexp.MustCompile(`"created_time":"(\d{10,13})"`),
		regexp.MustCompile(`"publish_time":"(\d{10,13})"`),
	}
	fbCreatedAtStringPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"creation_time":"([^"]+)"`),
		regexp.MustCompile(`"created_time":"([^"]+)"`),
		regexp.MustCompile(`"publish_time":"([^"]+)"`),
	}
	likesRe = []*regexp.Regexp{
		regexp.MustCompile(`"reaction_count":\{"count":(\d+)`),
		regexp.MustCompile(`"reactors":\{"count":(\d+)`),
	}
	commentsRe = []*regexp.Regexp{
		regexp.MustCompile(`"comment_count":\{"total_count":(\d+)`),
		regexp.MustCompile(`"comments":\{"total_count":(\d+)`),
	}
	sharesRe = []*regexp.Regexp{
		regexp.MustCompile(`"share_count":\{"count":(\d+)`),
		regexp.MustCompile(`"reshares":\{"count":(\d+)`),
	}
	viewsRe = []*regexp.Regexp{
		regexp.MustCompile(`"video_view_count":(\d+)`),
		regexp.MustCompile(`"play_count":(\d+)`),
		regexp.MustCompile(`"view_count":(\d+)`),
	}
)

// Extractor extracts media from Facebook posts and videos.
type Extractor struct{ client probe.Doer }

type fbMetadata struct {
	Title       string
	Author      string
	Description string
	CreatedAt   string
	Thumbnail   string
	Views       int64
	Likes       int64
	Comments    int64
	Shares      int64
}

type rawFormat struct {
	Quality string
	URL     string
}

const facebookUserAgent = "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1"

// NewExtractor creates a new Facebook extractor with the given HTTP client.
func NewExtractor(client probe.Doer) *Extractor {
	if client == nil {
		client = outbound.NewDefaultHTTPClient()
	}
	return &Extractor{client: client}
}

// Match returns true if the URL is a valid Facebook URL.
func Match(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return strings.Contains(host, "facebook.com") || strings.Contains(host, "fb.watch") || strings.Contains(host, "fb.me")
}

func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

// Extract extracts media metadata from a Facebook URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	html, finalURL, err := e.fetch(rawURL, opts, ctx)
	if err != nil {
		return nil, err
	}
	if err := e.checkLoginRequired(html); err != nil {
		return nil, err
	}
	metadata := e.extractMetadata(html, finalURL)
	formats := e.extractFormats(html)
	if isFacebookStoryURL(finalURL) {
		formats = preferStoryFormats(formats)
	}
	if len(formats) == 0 {
		return nil, extract.WrapCode(extract.KindExtractionFailed, extract.ErrCodeNoMedia, extract.ErrMsgNoMediaFound, false, nil)
	}
	mediaItems := buildMediaItems(ctx, e.client, formats, metadata, finalURL)
	contentType := "post"
	if isFacebookStoryURL(finalURL) {
		contentType = "post"
	} else if len(mediaItems) == 1 && mediaItems[0].Type == "video" {
		contentType = "video"
	} else if len(mediaItems) == 1 && mediaItems[0].Type == "image" {
		contentType = "image"
	}
	return extract.NewResultBuilder(rawURL, "facebook", "native").
		ContentType(contentType).
		Title(metadata.Title).
		Author(metadata.Author, "").
		Engagement(metadata.Views, metadata.Likes, metadata.Comments, metadata.Shares, 0).
		Media(mediaItems).
		Build(), nil
}

func buildMediaItems(ctx context.Context, client probe.Doer, formats []rawFormat, metadata fbMetadata, finalURL string) []extract.MediaItem {
	if len(formats) == 0 {
		return nil
	}
	allImages := true
	for _, format := range formats {
		if looksLikeVideoURL(format.URL) {
			allImages = false
			break
		}
	}
	if allImages {
		items := make([]extract.MediaItem, 0, len(formats))
		for i, format := range formats {
			container, mimeType := extract.ContainerAndMIMEFromURL(format.URL)
			urlValue := unescapeURL(format.URL)
			size := probe.SizeWithDoer(ctx, client, urlValue, map[string]string{"Referer": finalURL, "User-Agent": facebookUserAgent})
			thumbnail := strings.TrimSpace(metadata.Thumbnail)
			if thumbnail == "" {
				thumbnail = urlValue
			}
			items = append(items, extract.MediaItem{Index: i, Type: "image", ThumbnailURL: thumbnail, FileSizeBytes: size, Sources: []extract.MediaSource{{Quality: strings.TrimSpace(format.Quality), URL: urlValue, Referer: finalURL, Origin: extract.SourceOrigin(finalURL), MIMEType: mimeType, Protocol: protocolForFBURL(format.URL), Container: container, FileSizeBytes: size}}})
		}
		return items
	}
	media := extract.MediaItem{Index: 0, Type: "video", ThumbnailURL: metadata.Thumbnail, Sources: make([]extract.MediaSource, 0, len(formats))}
	for _, format := range formats {
		container, mimeType := extract.ContainerAndMIMEFromURL(format.URL)
		urlValue := unescapeURL(format.URL)
		size := probe.SizeWithDoer(ctx, client, urlValue, map[string]string{"Referer": finalURL, "User-Agent": facebookUserAgent})
		media.FileSizeBytes += size
		media.Sources = append(media.Sources, extract.MediaSource{Quality: strings.TrimSpace(format.Quality), URL: urlValue, Referer: finalURL, Origin: extract.SourceOrigin(finalURL), MIMEType: mimeType, Protocol: protocolForFBURL(format.URL), Container: container, FileSizeBytes: size, HasAudio: true, HasVideo: true, IsProgressive: container == "mp4"})
	}
	return []extract.MediaItem{media}
}

func (e *Extractor) fetch(rawURL string, opts extract.ExtractOptions, ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", facebookUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if opts.UseAuth && opts.CookieHeader != "" {
		req.Header.Set("Cookie", opts.CookieHeader)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return "", "", extract.Wrap(extract.KindUpstreamFailure, extract.ErrMsgUpstreamFailure, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = strings.TrimSpace(resp.Request.URL.String())
	}
	if finalURL == "" {
		finalURL = rawURL
	}
	return string(body), finalURL, nil
}

func (e *Extractor) checkLoginRequired(html string) error {
	lowerHTML := strings.ToLower(html)
	hasMedia := strings.Contains(html, "browser_native_hd_url") || strings.Contains(html, "browser_native_sd_url") || strings.Contains(html, "playable_url") || strings.Contains(html, "progressive_url") || strings.Contains(html, "all_subattachments") || strings.Contains(html, "viewer_image") || strings.Contains(html, "photo_image") || strings.Contains(html, "preferred_thumbnail") || strings.Contains(html, "story_thumbnail") || strings.Contains(html, "previewImage") || strings.Contains(html, "dash_manifest") || strings.Contains(html, "hd_src") || strings.Contains(html, "sd_src") || strings.Contains(html, "video_url")
	hasLogin := strings.Contains(lowerHTML, "login_form") || strings.Contains(lowerHTML, "log in to facebook") || strings.Contains(lowerHTML, "log in to continue") || strings.Contains(lowerHTML, "loginbutton") || strings.Contains(lowerHTML, "login.php")
	isSmallPage := len(html) < 100000
	isLoginRedirect := strings.Contains(lowerHTML, "/login/") || strings.Contains(lowerHTML, "login.php") || strings.Contains(lowerHTML, "next="+url.QueryEscape("/login"))
	if !hasMedia {
		for _, pattern := range []string{"content isn't available", "this content isn't available", "content unavailable", "only visible to friends", "you can't view this", "this content is private", "content is only visible"} {
			if strings.Contains(lowerHTML, pattern) {
				return extract.WrapCode(extract.KindExtractionFailed, "content_unavailable", "content isn't available", false, nil)
			}
		}
		for _, pattern := range []string{"page not found", "this page isn't available", "the link may be broken", "removed or expired", "link has expired"} {
			if strings.Contains(lowerHTML, pattern) {
				return extract.WrapCode(extract.KindExtractionFailed, "content_unavailable", "content isn't available", false, nil)
			}
		}
	}
	if !hasMedia && hasLogin && (isSmallPage || isLoginRedirect) {
		return extract.Wrap(extract.KindAuthRequired, extract.ErrMsgAuthRequired, fmt.Errorf("login required to access this content"))
	}
	for _, pattern := range []string{"age_restricted", "age restricted", "you must be 18", "you must be at least", "adult content", `"gating_type":"age_gating"`} {
		if strings.Contains(lowerHTML, pattern) {
			return extract.WrapCode(extract.KindAuthRequired, "age_restricted", extract.ErrMsgAuthRequired, false, nil)
		}
	}
	return nil
}

func (e *Extractor) extractMetadata(html, finalURL string) fbMetadata {
	m := fbMetadata{}
	isStory := isFacebookStoryURL(finalURL)
	titleRe := regexp.MustCompile(`<meta[^>]+property="og:title"[^>]+content="([^"]+)"`)
	descRe := regexp.MustCompile(`<meta[^>]+property="og:description"[^>]+content="([^"]+)"`)
	thumbRe := regexp.MustCompile(`<meta[^>]+property="og:image"[^>]+content="([^"]+)"`)
	if x := titleRe.FindStringSubmatch(html); len(x) > 1 {
		m.Title = normalizeTextField(x[1])
	}
	if x := descRe.FindStringSubmatch(html); len(x) > 1 {
		m.Description = normalizeTextField(x[1])
	}
	if x := thumbRe.FindStringSubmatch(html); len(x) > 1 {
		m.Thumbnail = unescapeURL(x[1])
	}
	if m.Thumbnail == "" {
		m.Thumbnail = extractInlineThumbnail(html)
	}
	if isStory {
		m.Author = extractFirstAuthor(html, fbStoryAuthorPatterns, isUsableStoryAuthor)
		if m.Author == "" {
			m.Author = extractStoryAuthorFromURL(finalURL)
		}
	}
	if m.Author == "" {
		m.Author = extractFirstAuthor(html, fbAuthorPatterns, isUsableGenericAuthor)
	}
	for _, re := range likesRe {
		if x := re.FindStringSubmatch(html); len(x) > 1 {
			m.Likes = extract.ParseHumanInt(x[1])
			break
		}
	}
	for _, re := range commentsRe {
		if x := re.FindStringSubmatch(html); len(x) > 1 {
			m.Comments = extract.ParseHumanInt(x[1])
			break
		}
	}
	for _, re := range sharesRe {
		if x := re.FindStringSubmatch(html); len(x) > 1 {
			m.Shares = extract.ParseHumanInt(x[1])
			break
		}
	}
	for _, re := range viewsRe {
		if x := re.FindStringSubmatch(html); len(x) > 1 {
			m.Views = extract.ParseHumanInt(x[1])
			break
		}
	}
	if m.Views == 0 || m.Likes == 0 {
		for _, text := range []string{m.Title, m.Description} {
			views, likes := parseStatsFromTitleLikeText(text)
			if m.Views == 0 {
				m.Views = views
			}
			if m.Likes == 0 {
				m.Likes = likes
			}
			if m.Views > 0 && m.Likes > 0 {
				break
			}
		}
	}
	m.Title = cleanFacebookTitle(m.Title, m.Author)
	m.CreatedAt = extractCreatedAt(html)
	if isStory && strings.TrimSpace(m.Title) == "" && strings.TrimSpace(m.Description) == "" {
		m.Title = "story"
	}
	return m
}

func (e *Extractor) extractFormats(html string) []rawFormat {
	formats := []rawFormat{}
	seenByURL := map[string]bool{}
	seenByCanonical := map[string]bool{}
	addFormat := func(quality, raw string) {
		quality = strings.TrimSpace(quality)
		if quality == "" {
			quality = "Original"
		}
		raw = unescapeURL(raw)
		if raw == "" {
			return
		}
		canonicalKey := normalizeQualityForDedup(quality) + "|" + canonicalURLForDedup(raw)
		if seenByURL[raw] || seenByCanonical[canonicalKey] {
			return
		}
		seenByURL[raw] = true
		seenByCanonical[canonicalKey] = true
		formats = append(formats, rawFormat{Quality: quality, URL: raw})
	}
	for _, pair := range []struct{ quality, pattern string }{
		{"HD", `"browser_native_hd_url":"(https:[^"]+)"`},
		{"SD", `"browser_native_sd_url":"(https:[^"]+)"`},
		{"HD", `"playable_url_quality_hd":"(https:[^"]+)"`},
		{"SD", `"playable_url":"(https:[^"]+)"`},
		{"HD", `"hd_src":"(https:[^"]+)"`},
		{"SD", `"sd_src":"(https:[^"]+)"`},
		{"Original", `"viewer_image":\{"height":\d+,"width":\d+,"uri":"(https:[^"]+)"`},
		{"Original", `"photo_image":\{"uri":"(https:[^"]+)"`},
		{"Original", `"preferred_thumbnail":\{"uri":"(https:[^"]+)"`},
		{"Original", `"(?:previewImage|story_thumbnail|poster_image)":\{"uri":"(https:[^"]+)"`},
	} {
		for _, match := range regexp.MustCompile(pair.pattern).FindAllStringSubmatch(html, -1) {
			if len(match) > 1 {
				addFormat(pair.quality, match[1])
			}
		}
	}
	qualityPattern := regexp.MustCompile(`"progressive_url":"(https:[^"]+\.mp4[^"]*)","failure_reason":null,"metadata":\{"quality":"(HD|SD)"\}`)
	for _, match := range qualityPattern.FindAllStringSubmatch(html, -1) {
		if len(match) > 2 {
			addFormat(match[2], match[1])
		}
	}
	genericPattern := regexp.MustCompile(`"progressive_url":"(https:[^"]+\.mp4[^"]*)"`)
	for _, match := range genericPattern.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			addFormat("HD", match[1])
		}
	}
	dashPattern := regexp.MustCompile(`"height":(\d+)[^}]*?"base_url":"(https:[^"]+\.mp4[^"]*)"`)
	for _, match := range dashPattern.FindAllStringSubmatch(html, -1) {
		if len(match) > 2 {
			addFormat(match[1]+"p", match[2])
		}
	}
	if len(formats) == 0 {
		subPattern := regexp.MustCompile(`"all_subattachments":\{[^}]*"nodes":\[(.+?)\]\}`)
		if subMatch := subPattern.FindStringSubmatch(html); len(subMatch) > 1 {
			uriPattern := regexp.MustCompile(`"uri":"(https:[^"]+)"`)
			for i, match := range uriPattern.FindAllStringSubmatch(subMatch[1], -1) {
				if len(match) > 1 {
					addFormat(fmt.Sprintf("Image %d", i+1), match[1])
				}
			}
		}
	}
	return formats
}

func preferStoryFormats(formats []rawFormat) []rawFormat {
	if len(formats) <= 1 {
		return formats
	}
	highestIdx, highestScore, lowestIdx, lowestScore := -1, -1, -1, int(^uint(0)>>1)
	for i, f := range formats {
		score := storyQualityScore(f.Quality)
		if score >= 720 && score > highestScore {
			highestScore = score
			highestIdx = i
		}
		if score > 0 && score < lowestScore {
			lowestScore = score
			lowestIdx = i
		}
	}
	if highestIdx >= 0 {
		return []rawFormat{formats[highestIdx]}
	}
	if lowestIdx >= 0 {
		return []rawFormat{formats[lowestIdx]}
	}
	return []rawFormat{formats[0]}
}

func storyQualityScore(quality string) int {
	q := strings.ToLower(strings.TrimSpace(quality))
	if q == "" {
		return 0
	}
	if match := fbQualityHeightPattern.FindStringSubmatch(q); len(match) > 1 {
		if n, err := strconv.Atoi(match[1]); err == nil {
			return n
		}
	}
	switch {
	case strings.Contains(q, "4k"), strings.Contains(q, "uhd"):
		return 2160
	case strings.Contains(q, "2k"), strings.Contains(q, "qhd"):
		return 1440
	case strings.Contains(q, "fhd"), strings.Contains(q, "full hd"), strings.Contains(q, "1080"):
		return 1080
	case strings.Contains(q, "hd"):
		return 720
	case strings.Contains(q, "sd"):
		return 480
	case strings.Contains(q, "ld"), strings.Contains(q, "low"):
		return 360
	case strings.Contains(q, "original"):
		return 1080
	default:
		return 0
	}
}
