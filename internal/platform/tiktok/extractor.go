package tiktok

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/netutil"
	"downaria-api/internal/outbound"
	"downaria-api/internal/platform/probe"
)

const apiURL = "https://www.tikwm.com/api/"

// Extractor extracts media from TikTok videos.
type Extractor struct{ client probe.Doer }

// NewExtractor creates a new TikTok extractor with the given HTTP client.
func NewExtractor(client probe.Doer) *Extractor {
	if client == nil {
		client = outbound.NewDefaultHTTPClient()
	}
	return &Extractor{client: client}
}

// Match returns true if the URL is a valid TikTok URL.
func Match(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return strings.Contains(host, "tiktok.com")
}

func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

// Extract extracts media metadata from a TikTok URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	form := url.Values{"url": []string{rawURL}, "hd": []string{"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, extract.Wrap(extract.KindInternal, "tiktok request init failed", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", defaultUserAgent)
	if opts.UseAuth && strings.TrimSpace(opts.CookieHeader) != "" && netutil.SameHost(rawURL, apiURL) {
		req.Header.Set("Cookie", opts.CookieHeader)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, extract.Wrap(extract.KindUpstreamFailure, extract.ErrMsgUpstreamFailure, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, extract.Wrap(extract.KindUpstreamFailure, fmt.Sprintf("unexpected tiktok status: %d", resp.StatusCode), nil)
	}
	reader := decodedBody(resp)
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Duration     int    `json:"duration"`
			Play         string `json:"play"`
			HDPlay       string `json:"hdplay"`
			PlayCount    int64  `json:"play_count"`
			DiggCount    int64  `json:"digg_count"`
			CommentCount int64  `json:"comment_count"`
			ShareCount   int64  `json:"share_count"`
			OriginCover  string `json:"origin_cover"`
			Author       struct {
				Nickname string `json:"nickname"`
				UniqueID string `json:"unique_id"`
			} `json:"author"`
		} `json:"data"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "invalid tiktok payload", err)
	}
	if payload.Code != 0 {
		return nil, extract.WrapCode(extract.KindExtractionFailed, "tiktok_extract_failed", extract.FirstNonEmpty(strings.TrimSpace(payload.Msg), "tiktok extraction failed"), false, nil)
	}
	videoURL := extract.FirstNonEmpty(strings.TrimSpace(payload.Data.HDPlay), strings.TrimSpace(payload.Data.Play))
	if videoURL == "" {
		return nil, extract.WrapCode(extract.KindExtractionFailed, extract.ErrCodeNoMedia, extract.ErrMsgNoMediaFound, false, nil)
	}
	size := probe.SizeWithDoer(ctx, e.client, videoURL, map[string]string{"Referer": rawURL, "User-Agent": defaultUserAgent})
	media := []extract.MediaItem{{Index: 0, Type: "video", ThumbnailURL: strings.TrimSpace(payload.Data.OriginCover), FileSizeBytes: size, Sources: []extract.MediaSource{{FormatID: "hd", Quality: "hd", URL: videoURL, Referer: rawURL, Origin: extract.SourceOrigin(rawURL), MIMEType: "video/mp4", Protocol: "https", Container: "mp4", DurationSeconds: float64(payload.Data.Duration), FileSizeBytes: size, HasAudio: true, HasVideo: true, IsProgressive: true}}}}
	return extract.NewResultBuilder(rawURL, "tiktok", "native").
		ContentType("video").
		Title(strings.TrimSpace(payload.Data.Title)).
		Author(strings.TrimSpace(payload.Data.Author.Nickname), strings.TrimSpace(payload.Data.Author.UniqueID)).
		Engagement(payload.Data.PlayCount, payload.Data.DiggCount, payload.Data.CommentCount, payload.Data.ShareCount, 0).
		Media(media).
		Build(), nil
}

func decodedBody(resp *http.Response) io.Reader {
	reader := io.Reader(resp.Body)
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		if gz, err := gzip.NewReader(resp.Body); err == nil {
			return gz
		}
	case "deflate":
		if zr, err := zlib.NewReader(resp.Body); err == nil {
			return zr
		}
	}
	return reader
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36"
