package pixiv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/outbound"
	"downaria-api/internal/platform/probe"
)

var artworkIDRegex = regexp.MustCompile(`artworks/(\d+)`)

// Extractor extracts media from Pixiv artworks.
type Extractor struct{ client probe.Getter }

// NewExtractor creates a new Pixiv extractor with the given HTTP client.
func NewExtractor(client probe.Getter) *Extractor {
	if client == nil {
		c := outbound.NewDefaultHTTPClient()
		client = probe.GetterFunc(func(ctx context.Context, rawURL string, headers map[string]string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
			if err != nil {
				return nil, err
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			return c.Do(req)
		})
	}
	return &Extractor{client: client}
}

// Match returns true if the URL is a valid Pixiv URL.
func Match(rawURL string) bool {
	p, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.Contains(strings.ToLower(p.Hostname()), "pixiv.net")
}
func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

// Extract extracts media metadata from a Pixiv URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	id := extractArtworkID(rawURL)
	if id == "" {
		return nil, extract.Wrap(extract.KindInvalidInput, extract.ErrMsgInvalidURL, nil)
	}
	apiURL := fmt.Sprintf("https://www.pixiv.net/ajax/illust/%s", id)
	headers := map[string]string{"Referer": "https://www.pixiv.net/", "User-Agent": defaultUserAgent}
	if opts.UseAuth && opts.CookieHeader != "" {
		headers["Cookie"] = opts.CookieHeader
	}
	resp, err := e.client.Get(ctx, apiURL, headers)
	if err != nil {
		return nil, extract.Wrap(extract.KindUpstreamFailure, extract.ErrMsgUpstreamFailure, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, extract.Wrap(extract.KindUpstreamFailure, fmt.Sprintf("unexpected pixiv status: %d", resp.StatusCode), nil)
	}
	var payload struct {
		Error   bool   `json:"error"`
		Message string `json:"message"`
		Body    struct {
			IllustTitle   string `json:"illustTitle"`
			UserName      string `json:"userName"`
			UserAccount   string `json:"userAccount"`
			LikeCount     int64  `json:"likeCount"`
			BookmarkCount int64  `json:"bookmarkCount"`
			ViewCount     int64  `json:"viewCount"`
			CommentCount  int64  `json:"commentCount"`
			PageCount     int    `json:"pageCount"`
			URLs          struct {
				Original string `json:"original"`
			} `json:"urls"`
		} `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "invalid pixiv payload", err)
	}
	if payload.Error {
		return nil, extract.WrapCode(extract.KindExtractionFailed, "pixiv_extract_failed", extract.FirstNonEmpty(strings.TrimSpace(payload.Message), "pixiv extraction failed"), false, nil)
	}
	if strings.TrimSpace(payload.Body.URLs.Original) == "" {
		return nil, extract.WrapCode(extract.KindExtractionFailed, extract.ErrCodeNoMedia, extract.ErrMsgNoMediaFound, false, nil)
	}
	items := make([]extract.MediaItem, 0, maxInt(1, payload.Body.PageCount))
	totalSize := int64(0)
	for i := 0; i < maxInt(1, payload.Body.PageCount); i++ {
		pageURL := payload.Body.URLs.Original
		if payload.Body.PageCount > 1 {
			pageURL = strings.Replace(pageURL, "_p0", fmt.Sprintf("_p%d", i), 1)
		}
		size := probe.SizeWithGetter(ctx, e.client, pageURL, headers)
		totalSize += size
		items = append(items, extract.MediaItem{Index: i, Type: "image", ThumbnailURL: pageURL, FileSizeBytes: size, Sources: []extract.MediaSource{{Quality: "original", URL: pageURL, Referer: "https://www.pixiv.net/", Origin: "https://www.pixiv.net", MIMEType: mimeTypeFromPixivURL(pageURL), Protocol: "https", Container: containerFromPixivURL(pageURL), FileSizeBytes: size}}})
	}
	return extract.NewResultBuilder(rawURL, "pixiv", "native").
		Title(strings.TrimSpace(payload.Body.IllustTitle)).
		Author(strings.TrimSpace(payload.Body.UserName), strings.TrimSpace(payload.Body.UserAccount)).
		Engagement(payload.Body.ViewCount, payload.Body.LikeCount, payload.Body.CommentCount, 0, payload.Body.BookmarkCount).
		Media(items).
		Build(), nil
}
func extractArtworkID(rawURL string) string {
	m := artworkIDRegex.FindStringSubmatch(rawURL)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
func mimeTypeFromPixivURL(rawURL string) string {
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(filepathExt(rawURL)), "."))
	switch ext {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
func containerFromPixivURL(rawURL string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(filepathExt(rawURL))), ".")
}
func filepathExt(rawURL string) string {
	p, _ := url.Parse(rawURL)
	path := rawURL
	if p != nil && p.Path != "" {
		path = p.Path
	}
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return ".jpg"
	}
	return path[idx:]
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36"
