package threads

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
)

var (
	postIDRegex       = regexp.MustCompile(`/@[^/]+/post/([A-Za-z0-9_-]+)`)
	authorRegex       = regexp.MustCompile(`/@([^/]+)/post/[A-Za-z0-9_-]+`)
	metaPropertyRegex = regexp.MustCompile(`(?is)<meta[^>]+property=["']([^"']+)["'][^>]+content=["']([^"']*)["'][^>]*>`)
	metaContentRegex  = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']*)["'][^>]+property=["']([^"']+)["'][^>]*>`)
	mediaURLRegex     = regexp.MustCompile(`https:(?:\\/\\/|//)[^"'\s<>]+(?:\.mp4|\.m3u8|\.jpg|\.jpeg|\.png|\.webp)(?:\\?[^"'\s<>]*)?`)

	likesCountRegex    = regexp.MustCompile(`(?is)aria-label=["'](?:Suka|Like)["'][^>]*>.*?<span[^>]*>([0-9][0-9\.,KMBkmb]*)</span>`)
	commentsCountRegex = regexp.MustCompile(`(?is)aria-label=["'](?:Komentar|Comment)["'][^>]*>.*?<span[^>]*>([0-9][0-9\.,KMBkmb]*)</span>`)
	sharesCountRegex   = regexp.MustCompile(`(?is)aria-label=["'](?:Bagikan|Share|Kirim ulang|Repost)["'][^>]*>.*?<span[^>]*>([0-9][0-9\.,KMBkmb]*)</span>`)
	jsonNumberRegex    = regexp.MustCompile(`(?is)"([a-z_]+)"\s*:\s*(\d+)`)
)

var fetchProfiles = []map[string]string{
	{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.9,id;q=0.8",
		"Cache-Control":             "no-cache",
		"Pragma":                    "no-cache",
		"Upgrade-Insecure-Requests": "1",
		"Referer":                   "https://www.facebook.com/",
		"User-Agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
	},
	{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9,id;q=0.8",
		"Cache-Control":   "no-cache",
		"Pragma":          "no-cache",
		"Referer":         "https://www.facebook.com/",
		"User-Agent":      "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
	},
}

type ThreadsExtractor struct {
	*core.BaseExtractor
}

type mediaCandidate struct {
	url   string
	score int
}

func NewThreadsExtractor() *ThreadsExtractor {
	return &ThreadsExtractor{BaseExtractor: core.NewBaseExtractor()}
}

func (e *ThreadsExtractor) Match(urlStr string) bool {
	return isThreadsURL(urlStr)
}

func (e *ThreadsExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	normalizedURL, err := normalizeThreadsURL(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid threads URL: %w", err)
	}

	var body []byte
	resolvedURL := normalizedURL
	var videos, images []string
	var meta map[string]string
	var likes, comments, shares, views int64
	var selectedHeaders map[string]string

	for _, headers := range fetchProfiles {
		resp, reqErr := e.MakeRequest(http.MethodGet, normalizedURL, nil, opts, headers)
		if reqErr != nil {
			continue
		}

		if statusErr := e.CheckStatus(resp, http.StatusOK); statusErr != nil {
			_ = resp.Body.Close()
			continue
		}

		candidateBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			continue
		}

		candidateResolvedURL := normalizedURL
		if resp.Request != nil && resp.Request.URL != nil {
			candidateResolvedURL = resp.Request.URL.String()
		}

		candidateMeta := parseMetaProperties(candidateBody)
		candidateVideos, candidateImages := collectMediaURLs(candidateBody, candidateMeta)
		candidateLikes, candidateComments, candidateShares, candidateViews := parseEngagement(candidateBody)

		if len(candidateVideos)+len(candidateImages) > len(videos)+len(images) {
			body = candidateBody
			resolvedURL = candidateResolvedURL
			meta = candidateMeta
			videos = candidateVideos
			images = candidateImages
			likes = candidateLikes
			comments = candidateComments
			shares = candidateShares
			views = candidateViews
			selectedHeaders = headers
		}

		if len(videos)+len(images) > 0 {
			break
		}
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("threads fetch failed")
	}

	postID := util.ExtractFirstRegexGroup(resolvedURL, postIDRegex)
	handle := util.ExtractFirstRegexGroup(resolvedURL, authorRegex)
	title := strings.TrimSpace(meta["og:title"])
	description := strings.TrimSpace(meta["og:description"])
	thumbnail := bestThreadsThumbnail(meta, images)

	if len(videos) == 0 && len(images) == 0 {
		return nil, fmt.Errorf("threads media not found")
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("threads").
		WithMediaType(core.MediaTypePost).
		WithAuthor("", handle).
		WithContent(postID, pickFirstNonEmpty(title, description), pickFirstNonEmpty(description, title)).
		WithEngagement(views, likes, comments, shares).
		WithAuthentication(opts.Cookie != "", opts.Source)

	idx := 0
	for _, mediaURL := range videos {
		media := core.NewMedia(idx, core.MediaTypeVideo, thumbnail)
		variant := core.NewVideoVariant("HD", mediaURL)
		if size := e.probeContentLength(mediaURL, opts, selectedHeaders); size > 0 {
			variant = variant.WithSize(size)
		}
		core.AddVariant(&media, variant)
		builder.AddMedia(media)
		idx++
	}
	for _, mediaURL := range images {
		media := core.NewMedia(idx, core.MediaTypeImage, mediaURL)
		variant := core.NewImageVariant("Original", mediaURL)
		if size := e.probeContentLength(mediaURL, opts, selectedHeaders); size > 0 {
			variant = variant.WithSize(size)
		}
		core.AddVariant(&media, variant)
		builder.AddMedia(media)
		idx++
	}

	return builder.Build(), nil
}

func bestThreadsThumbnail(meta map[string]string, images []string) string {
	if v := normalizeMediaURLString(strings.TrimSpace(meta["og:image"])); isCandidateMediaURL(v) {
		return v
	}
	if len(images) > 0 {
		return images[0]
	}
	return ""
}

func isThreadsURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
	}
	return host == "threads.net" || host == "www.threads.net" || host == "threads.com" || host == "www.threads.com"
}

func normalizeThreadsURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(u.Scheme) == "" {
		u.Scheme = "https"
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host == "" || host == "threads.com" || host == "www.threads.com" {
		u.Host = "www.threads.net"
	}
	return u.String(), nil
}

func parseMetaProperties(body []byte) map[string]string {
	out := map[string]string{}
	for _, m := range metaPropertyRegex.FindAllSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(string(m[1])))
		val := strings.TrimSpace(htmlUnescape(string(m[2])))
		if key != "" && val != "" {
			out[key] = val
		}
	}
	for _, m := range metaContentRegex.FindAllSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(string(m[2])))
		val := strings.TrimSpace(htmlUnescape(string(m[1])))
		if key != "" && val != "" {
			if _, ok := out[key]; !ok {
				out[key] = val
			}
		}
	}
	return out
}

func collectMediaURLs(body []byte, meta map[string]string) ([]string, []string) {
	videos := map[string]mediaCandidate{}
	images := map[string]mediaCandidate{}

	for _, m := range mediaURLRegex.FindAll(body, -1) {
		u := normalizeMediaURLString(string(m))
		if !isCandidateMediaURL(u) {
			continue
		}
		lower := strings.ToLower(u)
		switch {
		case strings.Contains(lower, ".mp4") || strings.Contains(lower, ".m3u8"):
			upsertCandidate(videos, mediaKey(u), u, scoreVideoURL(u))
		case strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") || strings.Contains(lower, ".png") || strings.Contains(lower, ".webp"):
			upsertCandidate(images, mediaKey(u), u, scoreImageURL(u))
		}
	}

	if v := normalizeMediaURLString(meta["og:video"]); v != "" {
		upsertCandidate(videos, mediaKey(v), v, scoreVideoURL(v)+250)
	}
	if i := normalizeMediaURLString(meta["og:image"]); i != "" {
		upsertCandidate(images, mediaKey(i), i, scoreImageURL(i)+100)
	}

	videoList := sortedCandidates(videos)
	imageList := sortedCandidates(images)

	// For video posts, media area should prioritize playable video variants only.
	// This prevents cover/thumbnail variants from flooding the result as separate items.
	if len(videoList) > 0 {
		return videoList, nil
	}

	return videoList, imageList
}

func isCandidateMediaURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return false
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "static.cdninstagram.com") || strings.Contains(lower, "/rsrc.php") {
		return false
	}
	if strings.Contains(lower, "profile_pic") {
		return false
	}

	path := strings.ToLower(u.Path)
	if strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m3u8") {
		return true
	}
	if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".webp") {
		return true
	}

	return false
}

func upsertCandidate(store map[string]mediaCandidate, key, urlStr string, score int) {
	if key == "" || urlStr == "" {
		return
	}
	if existing, ok := store[key]; !ok || score > existing.score {
		store[key] = mediaCandidate{url: urlStr, score: score}
	}
}

func mediaKey(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	q := u.Query()
	if ig := strings.TrimSpace(q.Get("ig_cache_key")); ig != "" {
		return strings.ToLower(strings.TrimSpace(u.Host)) + strings.ToLower(strings.TrimSpace(u.Path)) + "|" + ig
	}
	return strings.ToLower(strings.TrimSpace(u.Host)) + strings.ToLower(strings.TrimSpace(u.Path))
}

func scoreVideoURL(urlStr string) int {
	score := 1000
	lower := strings.ToLower(urlStr)
	if strings.Contains(lower, "xpv_progressive") {
		score += 400
	}
	if strings.Contains(lower, "1280") {
		score += 250
	} else if strings.Contains(lower, "1080") {
		score += 220
	} else if strings.Contains(lower, "720") {
		score += 180
	} else if strings.Contains(lower, "480") {
		score += 120
	}
	if strings.Contains(lower, "dash") {
		score -= 100
	}
	return score
}

func scoreImageURL(urlStr string) int {
	score := 100
	lower := strings.ToLower(urlStr)
	if strings.Contains(lower, "profile_pic") {
		return -10000
	}
	if strings.Contains(lower, "ig_cache_key=") {
		score += 700
	}

	u, err := url.Parse(urlStr)
	if err == nil {
		if stp := strings.TrimSpace(u.Query().Get("stp")); stp != "" {
			score += parseMaxDimension(stp)
			if strings.Contains(stp, "_s") || strings.Contains(stp, "_p") {
				score -= 250
			}
		} else {
			score += 500
		}
	}

	if strings.Contains(lower, "_tt6") {
		score += 40
	}
	return score
}

func parseMaxDimension(stp string) int {
	maxDim := 0
	for _, m := range regexp.MustCompile(`([sp])(\d+)x(\d+)`).FindAllStringSubmatch(stp, -1) {
		if len(m) < 4 {
			continue
		}
		w, errW := strconv.Atoi(m[2])
		h, errH := strconv.Atoi(m[3])
		if errW != nil || errH != nil {
			continue
		}
		if w > maxDim {
			maxDim = w
		}
		if h > maxDim {
			maxDim = h
		}
	}
	return maxDim
}

func normalizeMediaURLString(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	v = strings.Trim(v, `"'`)
	v = strings.ReplaceAll(v, `\/`, `/`)
	v = strings.ReplaceAll(v, `\u00253D`, `%3D`)
	v = strings.ReplaceAll(v, `\u0026`, `&`)
	v = strings.ReplaceAll(v, `\u0025`, `%`)
	v = htmlUnescape(v)
	if strings.HasPrefix(v, "//") {
		v = "https:" + v
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return ""
	}
	return v
}

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCandidates(set map[string]mediaCandidate) []string {
	arr := make([]mediaCandidate, 0, len(set))
	for _, v := range set {
		arr = append(arr, v)
	}
	sort.SliceStable(arr, func(i, j int) bool {
		if arr[i].score == arr[j].score {
			return arr[i].url < arr[j].url
		}
		return arr[i].score > arr[j].score
	})
	out := make([]string, 0, len(arr))
	for _, c := range arr {
		out = append(out, c.url)
	}
	return out
}

func parseMetric(body []byte, re *regexp.Regexp) int64 {
	m := re.FindSubmatch(body)
	if len(m) < 2 {
		return 0
	}
	return parseHumanInt(string(m[1]))
}

func (e *ThreadsExtractor) probeContentLength(targetURL string, opts core.ExtractOptions, headers map[string]string) int64 {
	if strings.TrimSpace(targetURL) == "" {
		return 0
	}

	headResp, err := e.MakeRequest(http.MethodHead, targetURL, nil, opts, headers)
	if err == nil && headResp != nil {
		defer headResp.Body.Close()
		if size := parseContentLengthFromResponse(headResp); size > 0 {
			return size
		}
	}

	rangeHeaders := map[string]string{"Range": "bytes=0-0"}
	for k, v := range headers {
		rangeHeaders[k] = v
	}
	rangeResp, err := e.MakeRequest(http.MethodGet, targetURL, nil, opts, rangeHeaders)
	if err != nil || rangeResp == nil {
		return 0
	}
	defer rangeResp.Body.Close()

	if size := parseContentLengthFromResponse(rangeResp); size > 0 {
		return size
	}

	return parseTotalFromContentRange(rangeResp.Header.Get("Content-Range"))
}

func parseContentLengthFromResponse(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	cl := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if cl == "" {
		return 0
	}
	n, err := strconv.ParseInt(cl, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parseTotalFromContentRange(contentRange string) int64 {
	value := strings.TrimSpace(contentRange)
	if value == "" {
		return 0
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0
	}
	total := strings.TrimSpace(parts[1])
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parseEngagement(body []byte) (likes, comments, shares, views int64) {
	likes = parseMetric(body, likesCountRegex)
	comments = parseMetric(body, commentsCountRegex)
	shares = parseMetric(body, sharesCountRegex)

	for _, m := range jsonNumberRegex.FindAllSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(string(m[1])))
		n, err := strconv.ParseInt(strings.TrimSpace(string(m[2])), 10, 64)
		if err != nil || n <= 0 {
			continue
		}

		switch key {
		case "like_count", "likes", "likes_count":
			if n > likes {
				likes = n
			}
		case "comment_count", "comments", "comments_count":
			if n > comments {
				comments = n
			}
		case "reshare_count", "share_count", "shares", "repost_count":
			if n > shares {
				shares = n
			}
		case "play_count", "view_count", "views", "video_view_count":
			if n > views {
				views = n
			}
		}
	}

	return likes, comments, shares, views
}

func parseHumanInt(raw string) int64 {
	s := strings.TrimSpace(strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(raw, ",", ""), ".", "")))
	if s == "" {
		return 0
	}
	mult := int64(1)
	if strings.HasSuffix(s, "K") {
		mult = 1000
		s = strings.TrimSuffix(s, "K")
	} else if strings.HasSuffix(s, "M") {
		mult = 1000000
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "B") {
		mult = 1000000000
		s = strings.TrimSuffix(s, "B")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n * mult
}

func htmlUnescape(s string) string {
	out := html.UnescapeString(s)
	out = strings.ReplaceAll(out, `\u003C`, "<")
	out = strings.ReplaceAll(out, `\u003E`, ">")
	out = strings.ReplaceAll(out, `\u0026`, "&")
	out = strings.ReplaceAll(out, `\u002F`, "/")
	return out
}

func pickFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
