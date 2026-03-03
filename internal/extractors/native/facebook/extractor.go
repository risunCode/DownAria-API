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
	"time"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/infra/network"
)

var (
	// Author patterns (ported from Fetchtium_RE)
	fbAuthorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"name":"([^"]+)","enable_reels_tab_deeplink":true`),
		regexp.MustCompile(`"owning_profile":\{"__typename":"(?:User|Page)","name":"([^"]+)"`),
		regexp.MustCompile(`"owner":\{"__typename":"(?:User|Page)"[^}]*"name":"([^"]+)"`),
		regexp.MustCompile(`"actors":\[\{"__typename":"User","name":"([^"]+)"`),
	}

	// Engagement patterns
	fbLikesPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"reaction_count":\{"count":(\d+)`),
		regexp.MustCompile(`"i18n_reaction_count":"([^"]+)"`),
		regexp.MustCompile(`"reactors":\{"count":(\d+)`),
		regexp.MustCompile(`"likecount":(\d+)`),
		regexp.MustCompile(`"like_count":(\d+)`),
	}

	fbCommentsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"comment_count":\{"total_count":(\d+)`),
		regexp.MustCompile(`"comments":\{"total_count":(\d+)`),
		regexp.MustCompile(`"total_comment_count":(\d+)`),
		regexp.MustCompile(`"commentcount":(\d+)`),
	}

	fbSharesPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"share_count":\{"count":(\d+)`),
		regexp.MustCompile(`"reshares":\{"count":(\d+)`),
		regexp.MustCompile(`"i18n_share_count":"([^"]+)"`),
		regexp.MustCompile(`"sharecount":(\d+)`),
	}

	fbViewsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`"video_view_count":(\d+)`),
		regexp.MustCompile(`"play_count":(\d+)`),
		regexp.MustCompile(`"view_count":(\d+)`),
		regexp.MustCompile(`"viewCount":(\d+)`),
		regexp.MustCompile(`(\d+)\s*(?:views|Views|lượt xem|tayangan|次观看|回視聴|просмотр)`),
	}

	// Title fallback patterns (e.g. "83K views · 1.3K reactions | ...")
	fbTitleViewsPattern     = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?\s*[kmb]?)\s*views\b`)
	fbTitleLikesPattern     = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?\s*[kmb]?)\s*(?:reactions?|likes?)\b`)
	fbTitleStatsPrefixRegex = regexp.MustCompile(`(?i)^\s*(?:[0-9]+(?:[.,][0-9]+)?\s*[kmb]?\s*(?:views?|reactions?|likes?)\s*(?:·\s*)?)+\|\s*`)
	fbTitleSeparatorRegex   = regexp.MustCompile(`\s*([|·])\s*`)
)

// FacebookExtractor handles Facebook media extraction using HTML scraping
type FacebookExtractor struct {
	*core.BaseExtractor
}

func NewFacebookExtractor() *FacebookExtractor {
	return &FacebookExtractor{
		BaseExtractor: core.NewBaseExtractor(),
	}
}

func (e *FacebookExtractor) Match(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "facebook.com") || host == "fb.watch" || host == "fb.me" || strings.Contains(host, "fb.watch") || strings.Contains(host, "fb.me")
}

func (e *FacebookExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	// Use fresh client for Facebook (bypasses any shared state issues)
	html, finalURL, err := e.fetchWithFreshClient(urlStr, opts)
	if err != nil {
		return nil, err
	}

	if err := e.checkContentIssues(html); err != nil {
		return nil, err
	}

	if err := e.checkLoginRequired(html); err != nil {
		return nil, err
	}

	metadata := e.extractMetadata(html, finalURL)
	formats := e.extractFormats(html)

	if len(formats) == 0 {
		return nil, fmt.Errorf("no media found on page")
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("facebook").
		WithMediaType(e.detectMediaType(finalURL)).
		WithAuthor(metadata.Author, "").
		WithContent("", metadata.Title, metadata.Description).
		WithEngagement(metadata.Views, metadata.Likes, metadata.Comments, metadata.Shares).
		WithAuthentication(opts.Cookie != "", opts.Source)

	isImage := !strings.Contains(formats[0].URL, ".mp4") &&
		!strings.Contains(formats[0].URL, ".webm") &&
		!strings.Contains(formats[0].URL, "video")
	mediaType := core.MediaTypeVideo
	if isImage {
		mediaType = core.MediaTypeImage
	}

	media := core.NewMedia(0, mediaType, metadata.Thumbnail)
	for _, f := range formats {
		variant := core.NewVideoVariant(f.Quality, f.URL)
		if isImage {
			variant = core.NewImageVariant(f.Quality, f.URL)
		}
		ext := "mp4"
		if isImage {
			ext = "jpg"
		}
		filename := core.GenerateFilename(metadata.Author, metadata.Title, "", ext)
		variant = variant.WithFilename(filename)
		core.AddVariant(&media, variant)
	}
	builder.AddMedia(media)

	return builder.Build(), nil
}

func (e *FacebookExtractor) fetchWithFreshClient(urlStr string, opts core.ExtractOptions) (string, string, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	if opts.Cookie != "" {
		req.Header.Set("Cookie", opts.Cookie)
	}

	client := network.GetClientWithTimeout(30 * time.Second)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	finalURL := strings.TrimSpace(resp.Request.URL.String())
	if finalURL == "" {
		finalURL = urlStr
	}

	return string(body), finalURL, nil
}

func (e *FacebookExtractor) checkContentIssues(html string) error {
	lowerHTML := strings.ToLower(html)

	// Check if media is available first
	hasMedia := strings.Contains(html, "browser_native_hd_url") ||
		strings.Contains(html, "browser_native_sd_url") ||
		strings.Contains(html, "playable_url") ||
		strings.Contains(html, "all_subattachments") ||
		strings.Contains(html, "viewer_image") ||
		strings.Contains(html, "photo_image")

	// Check for content unavailable (cookie/private/age-restricted unified detection)
	// These three cases result in the same pattern: no media + login redirect + unavailable message
	contentUnavailablePatterns := []string{
		"content isn't available",
		"this content isn't available",
		"content unavailable",
	}
	hasContentUnavailable := false
	for _, pattern := range contentUnavailablePatterns {
		if strings.Contains(lowerHTML, pattern) {
			hasContentUnavailable = true
			break
		}
	}

	// Check for login redirect indicators
	loginRedirectPatterns := []string{
		"login.php",
		"login_form",
		"log in to facebook",
	}
	hasLoginRedirect := false
	for _, pattern := range loginRedirectPatterns {
		if strings.Contains(lowerHTML, pattern) {
			hasLoginRedirect = true
			break
		}
	}

	// UNIFIED ERROR: No media + content unavailable + login redirect = Cookie/Private/Age-Restricted
	// These three cases are indistinguishable without authentication, so we use one unified error
	if !hasMedia && hasContentUnavailable && hasLoginRedirect {
		return fmt.Errorf("login required")
	}

	// Age restricted specific patterns (detectable even with some content)
	ageRestrictedPatterns := []string{
		"age_restricted",
		"age restricted",
		"you must be 18",
		"you must be at least",
		"adult content",
		`"gating_type":"age_gating"`,
	}
	for _, pattern := range ageRestrictedPatterns {
		if strings.Contains(lowerHTML, pattern) {
			return fmt.Errorf("login required")
		}
	}

	// Only flag as private if specific privacy messages are found AND no media is available
	// The word "private" alone can appear in many contexts (e.g., "private group")
	privatePatterns := []string{
		"only visible to friends",
		"you can't view this",
		"this content is private",
		"content is only visible",
	}

	// Only reject as private if no media found AND private message found
	if !hasMedia {
		for _, pattern := range privatePatterns {
			if strings.Contains(lowerHTML, pattern) {
				return fmt.Errorf("content is private")
			}
		}
	}

	deletedPatterns := []string{
		"page not found",
		"this page isn't available",
		"the link may be broken",
		"removed or expired",
		"link has expired",
	}
	for _, pattern := range deletedPatterns {
		if strings.Contains(lowerHTML, pattern) {
			return fmt.Errorf("content has been deleted or is unavailable")
		}
	}

	return nil
}

func (e *FacebookExtractor) checkLoginRequired(html string) error {
	lowerHTML := strings.ToLower(html)

	mediaPatterns := []string{
		"browser_native_hd_url",
		"browser_native_sd_url",
		"playable_url",
		"all_subattachments",
		"viewer_image",
		"photo_image",
		"preferred_thumbnail",
		"hd_src",
		"sd_src",
		"video_url",
	}

	hasMedia := false
	for _, pattern := range mediaPatterns {
		if strings.Contains(html, pattern) {
			hasMedia = true
			break
		}
	}

	loginPatterns := []string{
		"login_form",
		"log in to facebook",
		"log in to continue",
		"loginbutton",
		"login.php",
	}

	hasLogin := false
	for _, pattern := range loginPatterns {
		if strings.Contains(lowerHTML, pattern) {
			hasLogin = true
			break
		}
	}

	isSmallPage := len(html) < 100000
	isLoginRedirect := strings.Contains(lowerHTML, "/login/") ||
		strings.Contains(lowerHTML, "login.php") ||
		strings.Contains(lowerHTML, "next="+url.QueryEscape("/login"))

	if !hasMedia && hasLogin && (isSmallPage || isLoginRedirect) {
		return fmt.Errorf("login required to access this content")
	}

	return nil
}

type fbMetadata struct {
	Title       string
	Author      string
	Description string
	Thumbnail   string
	Views       int64
	Likes       int64
	Comments    int64
	Shares      int64
}

func (e *FacebookExtractor) extractMetadata(html, finalURL string) fbMetadata {
	m := fbMetadata{}

	patterns := map[string]*regexp.Regexp{
		"title":       regexp.MustCompile(`<meta[^>]+property="og:title"[^>]+content="([^"]+)"`),
		"description": regexp.MustCompile(`<meta[^>]+property="og:description"[^>]+content="([^"]+)"`),
		"thumbnail":   regexp.MustCompile(`<meta[^>]+property="og:image"[^>]+content="([^"]+)"`),
	}

	if match := patterns["title"].FindStringSubmatch(html); len(match) > 1 {
		m.Title = normalizeTextField(match[1])
	}
	if match := patterns["description"].FindStringSubmatch(html); len(match) > 1 {
		m.Description = normalizeTextField(match[1])
	}
	if match := patterns["thumbnail"].FindStringSubmatch(html); len(match) > 1 {
		m.Thumbnail = unescapeHTML(match[1])
	}

	// Author patterns (HTML embedded JSON)
	for _, pattern := range fbAuthorPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			author := normalizeTextField(match[1])
			if author != "" && !strings.EqualFold(author, "facebook") {
				m.Author = author
				break
			}
		}
	}

	// Extract author from title if possible
	if strings.Contains(m.Title, " - ") {
		parts := strings.Split(m.Title, " - ")
		if len(parts) >= 2 {
			if m.Author == "" {
				m.Author = strings.TrimSpace(parts[0])
			}
		}
	}

	// Engagement extraction
	for _, pattern := range fbLikesPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			m.Likes = parseEngagement(match[1])
			break
		}
	}
	for _, pattern := range fbCommentsPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			m.Comments = parseEngagement(match[1])
			break
		}
	}
	for _, pattern := range fbSharesPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			m.Shares = parseEngagement(match[1])
			break
		}
	}
	for _, pattern := range fbViewsPatterns {
		if match := pattern.FindStringSubmatch(html); len(match) > 1 {
			m.Views = parseEngagement(match[1])
			break
		}
	}

	// Fallback to title-like text when direct regex extraction misses values.
	if m.Views == 0 || m.Likes == 0 {
		for _, text := range []string{m.Title, m.Description} {
			views, likes := parseStatsFromTitleLikeText(text)
			if m.Views == 0 && views > 0 {
				m.Views = views
			}
			if m.Likes == 0 && likes > 0 {
				m.Likes = likes
			}
			if m.Views > 0 && m.Likes > 0 {
				break
			}
		}
	}

	m.Title = cleanFacebookTitle(m.Title, m.Author)

	return m
}

func parseEngagement(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}

	// Remove quotes artifacts / separators
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")

	// Fast path: plain integer
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		if v < 0 {
			return 0
		}
		return v
	}

	// Mixed text fallback (e.g. "1.3K reactions")
	inlineRe := regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)([kmb]?)`)
	if m := inlineRe.FindStringSubmatch(s); len(m) > 1 {
		value := m[1]
		suffix := ""
		if len(m) > 2 {
			suffix = strings.ToUpper(m[2])
		}

		mult := float64(1)
		switch suffix {
		case "K":
			mult = 1_000
		case "M":
			mult = 1_000_000
		case "B":
			mult = 1_000_000_000
		}

		if f, err := strconv.ParseFloat(value, 64); err == nil {
			v := int64(f * mult)
			if v < 0 {
				return 0
			}
			return v
		}
	}

	// Handle suffix K/M/B (case-insensitive) and decimal values, e.g. 1.2K
	last := s[len(s)-1]
	mult := float64(1)
	if last == 'K' || last == 'k' {
		mult = 1_000
		s = s[:len(s)-1]
	} else if last == 'M' || last == 'm' {
		mult = 1_000_000
		s = s[:len(s)-1]
	} else if last == 'B' || last == 'b' {
		mult = 1_000_000_000
		s = s[:len(s)-1]
	}

	// Some i18n strings may include text; extract leading number token
	numRe := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?`)
	if m := numRe.FindString(s); m != "" {
		s = m
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	v := int64(f * mult)
	if v < 0 {
		return 0
	}
	return v
}

func parseStatsFromTitleLikeText(text string) (int64, int64) {
	if strings.TrimSpace(text) == "" {
		return 0, 0
	}

	views := int64(0)
	likes := int64(0)

	if match := fbTitleViewsPattern.FindStringSubmatch(text); len(match) > 1 {
		views = parseEngagement(strings.ReplaceAll(match[1], ",", ""))
	}
	if match := fbTitleLikesPattern.FindStringSubmatch(text); len(match) > 1 {
		likes = parseEngagement(strings.ReplaceAll(match[1], ",", ""))
	}

	return views, likes
}

func cleanFacebookTitle(title, author string) string {
	clean := strings.TrimSpace(title)
	if clean == "" {
		return ""
	}

	clean = fbTitleSeparatorRegex.ReplaceAllString(clean, ` $1 `)
	clean = strings.Join(strings.Fields(clean), " ")

	clean = fbTitleStatsPrefixRegex.ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)

	if author != "" {
		if idx := strings.LastIndex(clean, "|"); idx >= 0 {
			suffix := strings.TrimSpace(clean[idx+1:])
			if suffix != "" && strings.EqualFold(suffix, strings.TrimSpace(author)) {
				clean = strings.TrimSpace(clean[:idx])
			}
		}
	}

	clean = fbTitleSeparatorRegex.ReplaceAllString(clean, ` $1 `)
	clean = strings.Join(strings.Fields(clean), " ")

	return strings.TrimSpace(clean)
}

type rawFormat struct {
	Quality string
	URL     string
}

func (e *FacebookExtractor) extractFormats(html string) []rawFormat {
	formats := []rawFormat{}
	seen := make(map[string]bool)

	addFormat := func(quality, url string) {
		url = unescapeURL(url)
		if url == "" || seen[url] {
			return
		}
		seen[url] = true
		formats = append(formats, rawFormat{Quality: quality, URL: url})
	}

	// Priority 1: browser_native_hd_url and browser_native_sd_url
	if matches := regexp.MustCompile(`"browser_native_hd_url":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
		addFormat("HD", matches[1])
	}
	if matches := regexp.MustCompile(`"browser_native_sd_url":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
		addFormat("SD", matches[1])
	}

	// Priority 2: playable_url_quality_hd and playable_url
	if len(formats) == 0 {
		if matches := regexp.MustCompile(`"playable_url_quality_hd":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("HD", matches[1])
		}
		if matches := regexp.MustCompile(`"playable_url":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("SD", matches[1])
		}
	}

	// Priority 3: hd_src and sd_src
	if len(formats) == 0 {
		if matches := regexp.MustCompile(`"hd_src":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("HD", matches[1])
		}
		if matches := regexp.MustCompile(`"sd_src":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("SD", matches[1])
		}
	}

	// Priority 4: DASH formats with height and base_url
	if len(formats) == 0 {
		dashPattern := regexp.MustCompile(`"height":(\d+)[^}]*?"base_url":"(https:[^"]+\.mp4[^"]*)"`)
		matches := dashPattern.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) > 2 {
				quality := fmt.Sprintf("%sp", match[1])
				addFormat(quality, match[2])
			}
		}
	}

	// Image extraction
	if len(formats) == 0 {
		// viewer_image pattern
		if matches := regexp.MustCompile(`"viewer_image":\{"height":\d+,"width":\d+,"uri":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("Original", matches[1])
		}

		// photo_image pattern
		if matches := regexp.MustCompile(`"photo_image":\{"uri":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("Original", matches[1])
		}

		// preferred_thumbnail
		if matches := regexp.MustCompile(`"preferred_thumbnail":\{"uri":"(https:[^"]+)"`).FindStringSubmatch(html); len(matches) > 1 {
			addFormat("Original", matches[1])
		}

		// all_subattachments for galleries
		subPattern := regexp.MustCompile(`"all_subattachments":\{[^}]*"nodes":\[(.+?)\]\}`)
		if subMatch := subPattern.FindStringSubmatch(html); len(subMatch) > 1 {
			uriPattern := regexp.MustCompile(`"uri":"(https:[^"]+)"`)
			uriMatches := uriPattern.FindAllStringSubmatch(subMatch[1], -1)
			for i, uriMatch := range uriMatches {
				if len(uriMatch) > 1 {
					quality := fmt.Sprintf("Image %d", i+1)
					addFormat(quality, uriMatch[1])
				}
			}
		}
	}

	return formats
}

func unescapeURL(raw string) string {
	value := raw
	// JSON escaped sequences
	value = strings.ReplaceAll(value, `\/`, `/`)
	value = strings.ReplaceAll(value, `\\/`, `/`)
	value = strings.ReplaceAll(value, `\u003A`, `:`)
	value = strings.ReplaceAll(value, `\u002F`, `/`)
	value = strings.ReplaceAll(value, `\u003B`, `;`)
	value = strings.ReplaceAll(value, `\u003D`, `=`)
	value = strings.ReplaceAll(value, `\u0026`, `&`)
	value = strings.ReplaceAll(value, `\u003F`, `?`)
	value = strings.ReplaceAll(value, `\u0025`, `%`)
	value = strings.ReplaceAll(value, `\u0023`, `#`)
	value = strings.ReplaceAll(value, `\u0024`, `$`)
	value = strings.ReplaceAll(value, `\u0040`, `@`)

	// HTML entities
	value = strings.ReplaceAll(value, `&quot;`, `"`)
	value = strings.ReplaceAll(value, `&amp;`, `&`)
	value = strings.ReplaceAll(value, `&lt;`, `<`)
	value = strings.ReplaceAll(value, `&gt;`, `>`)
	value = strings.ReplaceAll(value, `&#x3D;`, `=`)
	value = strings.ReplaceAll(value, `&#x2F;`, `/`)

	return value
}

func unescapeHTML(raw string) string {
	value := raw

	// Handle hexadecimal numeric entities: &#xABCD;
	hexPattern := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	value = hexPattern.ReplaceAllStringFunc(value, func(match string) string {
		hexStr := match[3 : len(match)-1] // Extract hex digits
		if code, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
			return string(rune(code))
		}
		return match
	})

	// Handle decimal numeric entities: &#1234;
	decPattern := regexp.MustCompile(`&#(\d+);`)
	value = decPattern.ReplaceAllStringFunc(value, func(match string) string {
		decStr := match[2 : len(match)-1] // Extract decimal digits
		if code, err := strconv.ParseInt(decStr, 10, 32); err == nil {
			return string(rune(code))
		}
		return match
	})

	// Basic HTML entities
	value = strings.ReplaceAll(value, `&quot;`, `"`)
	value = strings.ReplaceAll(value, `&amp;`, `&`)
	value = strings.ReplaceAll(value, `&lt;`, `<`)
	value = strings.ReplaceAll(value, `&gt;`, `>`)
	value = strings.ReplaceAll(value, "&apos;", "'")

	return value
}

func decodeJSONStringEscapes(raw string) string {
	if raw == "" || !strings.Contains(raw, `\`) {
		return raw
	}

	quoted := `"` + strings.ReplaceAll(raw, `"`, `\"`) + `"`
	decoded, err := strconv.Unquote(quoted)
	if err != nil {
		return raw
	}

	return decoded
}

func normalizeTextField(raw string) string {
	value := strings.TrimSpace(raw)
	value = unescapeHTML(value)
	value = decodeJSONStringEscapes(value)
	return strings.TrimSpace(value)
}

func (e *FacebookExtractor) detectMediaType(urlStr string) core.MediaType {
	lowerURL := strings.ToLower(urlStr)
	switch {
	case strings.Contains(lowerURL, "/stories/"):
		return core.MediaTypeStory
	case strings.Contains(lowerURL, "/reel/"):
		return core.MediaTypeReel
	case strings.Contains(lowerURL, "/photo"):
		return core.MediaTypeImage
	default:
		return core.MediaTypeVideo
	}
}
