package threads

import (
	"context"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/outbound"
	"downaria-api/internal/platform/probe"
)

var (
	postIDRegex        = regexp.MustCompile(`/@[^/]+/post/([A-Za-z0-9_-]+)`)
	authorRegex        = regexp.MustCompile(`/@([^/]+)/post/[A-Za-z0-9_-]+`)
	metaPropertyRegex  = regexp.MustCompile(`(?is)<meta[^>]+property=["']([^"']+)["'][^>]+content=["']([^"']*)["'][^>]*>`)
	metaContentRegex   = regexp.MustCompile(`(?is)<meta[^>]+content=["']([^"']*)["'][^>]+property=["']([^"']+)["'][^>]*>`)
	mediaURLRegex      = regexp.MustCompile(`https:(?:\\/\\/|//)[^"'\s<>]+(?:\.mp4|\.m3u8|\.jpg|\.jpeg|\.png|\.webp)(?:\?[^"'\s<>]*)?`)
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
		"User-Agent":                defaultUserAgent,
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

// Extractor extracts media from Threads posts.
type Extractor struct{ client probe.Getter }
type mediaCandidate struct {
	url   string
	score int
}

// NewExtractor creates a new Threads extractor with the given HTTP client.
func NewExtractor(client probe.Getter) *Extractor {
	if client == nil {
		client = probe.GetterFunc(defaultGet)
	}
	return &Extractor{client: client}
}

// Match returns true if the URL is a valid Threads URL.
func Match(rawURL string) bool                { return isThreadsURL(rawURL) }
func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

// Extract extracts media metadata from a Threads URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedURL, err := normalizeThreadsURL(rawURL)
	if err != nil {
		return nil, extract.Wrap(extract.KindInvalidInput, extract.ErrMsgInvalidURL, err)
	}
	var (
		body        []byte
		resolvedURL = normalizedURL
		meta        map[string]string
		videos      []string
		images      []string
		likes       int64
		comments    int64
		shares      int64
		views       int64
		bestHeaders map[string]string
	)
	for _, headers := range fetchProfiles {
		resp, err := e.client.Get(ctx, normalizedURL, headers)
		if err != nil {
			continue
		}
		if resp.StatusCode != http.StatusOK {
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
			bestHeaders = headers
		}
		if len(videos)+len(images) > 0 {
			break
		}
	}
	if len(body) == 0 {
		return nil, extract.Wrap(extract.KindUpstreamFailure, extract.ErrMsgUpstreamFailure, nil)
	}
	if len(videos) == 0 && len(images) == 0 {
		return nil, extract.WrapCode(extract.KindExtractionFailed, extract.ErrCodeNoMedia, extract.ErrMsgNoMediaFound, false, nil)
	}
	title := strings.TrimSpace(meta["og:title"])
	description := strings.TrimSpace(meta["og:description"])
	thumbnail := bestThreadsThumbnail(meta, images)
	_ = extractFirstRegexGroup(resolvedURL, postIDRegex)
	handle := extractFirstRegexGroup(resolvedURL, authorRegex)
	items := make([]extract.MediaItem, 0, len(videos)+len(images))
	idx := 0
	for _, mediaURL := range videos {
		container, mimeType := extract.ContainerAndMIMEFromURL(mediaURL)
		size := probe.SizeWithGetter(ctx, e.client, mediaURL, bestHeaders)
		items = append(items, extract.MediaItem{Index: idx, Type: "video", ThumbnailURL: thumbnail, FileSizeBytes: size, Sources: []extract.MediaSource{{Quality: videoQuality(mediaURL), URL: mediaURL, Referer: resolvedURL, Origin: extract.SourceOrigin(resolvedURL), MIMEType: mimeType, Protocol: protocolFromURL(mediaURL), Container: container, FileSizeBytes: size, HasAudio: container == "mp4", HasVideo: true, IsProgressive: strings.HasSuffix(strings.ToLower(strings.Split(mediaURL, "?")[0]), ".mp4")}}})
		idx++
	}
	for _, mediaURL := range images {
		container, mimeType := extract.ContainerAndMIMEFromURL(mediaURL)
		size := probe.SizeWithGetter(ctx, e.client, mediaURL, bestHeaders)
		items = append(items, extract.MediaItem{Index: idx, Type: "image", ThumbnailURL: mediaURL, FileSizeBytes: size, Sources: []extract.MediaSource{{Quality: "original", URL: mediaURL, Referer: resolvedURL, Origin: extract.SourceOrigin(resolvedURL), MIMEType: mimeType, Protocol: protocolFromURL(mediaURL), Container: container, FileSizeBytes: size}}})
		idx++
	}
	return extract.NewResultBuilder(rawURL, "threads", "native").
		ContentType(contentType(items)).
		Title(extract.FirstNonEmpty(title, description)).
		Author("", handle).
		Engagement(views, likes, comments, shares, 0).
		Media(items).
		Build(), nil
}

func defaultGet(ctx context.Context, rawURL string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return outbound.NewDefaultHTTPClient().Do(req)
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
	if u.Scheme == "" {
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
		if len(m) >= 3 {
			out[strings.ToLower(strings.TrimSpace(string(m[1])))] = strings.TrimSpace(html.UnescapeString(string(m[2])))
		}
	}
	for _, m := range metaContentRegex.FindAllSubmatch(body, -1) {
		if len(m) >= 3 {
			key := strings.ToLower(strings.TrimSpace(string(m[2])))
			if _, ok := out[key]; !ok {
				out[key] = strings.TrimSpace(html.UnescapeString(string(m[1])))
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
	if v := normalizeMediaURLString(strings.TrimSpace(meta["og:video"])); v != "" {
		upsertCandidate(videos, mediaKey(v), v, scoreVideoURL(v)+250)
	}
	if i := normalizeMediaURLString(strings.TrimSpace(meta["og:image"])); i != "" {
		upsertCandidate(images, mediaKey(i), i, scoreImageURL(i)+100)
	}
	videoList := sortedCandidates(videos)
	imageList := sortedCandidates(images)
	if len(videoList) > 0 {
		return videoList, nil
	}
	return nil, imageList
}
func normalizeMediaURLString(value string) string {
	v := strings.TrimSpace(value)
	v = strings.Trim(v, `"'`)
	v = strings.ReplaceAll(v, `\/`, `/`)
	v = strings.ReplaceAll(v, `\u0026`, `&`)
	v = strings.ReplaceAll(v, `\u0025`, `%`)
	v = html.UnescapeString(v)
	if strings.HasPrefix(v, "//") {
		v = "https:" + v
	}
	if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
		return ""
	}
	return v
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
	if strings.Contains(lower, "static.cdninstagram.com") || strings.Contains(lower, "/rsrc.php") || strings.Contains(lower, "profile_pic") {
		return false
	}
	path := strings.ToLower(u.Path)
	return strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m3u8") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") || strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".webp")
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
	if ig := strings.TrimSpace(u.Query().Get("ig_cache_key")); ig != "" {
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
	for _, tok := range []string{"1280", "1080", "720", "480"} {
		if strings.Contains(lower, tok) {
			n, _ := strconv.Atoi(tok)
			score += n / 4
			break
		}
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
	return score
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
	return extract.ParseHumanInt(string(m[1]))
}
func parseEngagement(body []byte) (int64, int64, int64, int64) {
	likes := parseMetric(body, likesCountRegex)
	comments := parseMetric(body, commentsCountRegex)
	shares := parseMetric(body, sharesCountRegex)
	views := int64(0)
	for _, m := range jsonNumberRegex.FindAllSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		key := strings.ToLower(string(m[1]))
		n, _ := strconv.ParseInt(string(m[2]), 10, 64)
		switch key {
		case "play_count", "view_count":
			if n > views {
				views = n
			}
		case "like_count":
			if n > likes {
				likes = n
			}
		case "comment_count":
			if n > comments {
				comments = n
			}
		case "reshare_count", "share_count":
			if n > shares {
				shares = n
			}
		}
	}
	return likes, comments, shares, views
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
func extractFirstRegexGroup(value string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(value)
	if len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
func protocolFromURL(rawURL string) string {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if strings.Contains(lower, ".m3u8") {
		return "m3u8_native"
	}
	return "https"
}
func videoQuality(rawURL string) string {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	for _, tok := range []string{"1080p", "720p", "480p"} {
		if strings.Contains(lower, tok[:len(tok)-1]) || strings.Contains(lower, tok) {
			return tok
		}
	}
	if strings.Contains(lower, ".m3u8") {
		return "hls"
	}
	return "hd"
}
func contentType(items []extract.MediaItem) string {
	if len(items) == 1 {
		return items[0].Type
	}
	return "post"
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0"
