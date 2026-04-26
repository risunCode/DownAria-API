package instagram

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

const (
	graphQLAPI   = "https://www.instagram.com/graphql/query"
	graphQLDocID = "8845758582119845"
)

var shortcodeRegex = regexp.MustCompile(`/(?:p|reels?|tv)/([A-Za-z0-9_-]+)`)

// Extractor extracts media from Instagram posts.
type Extractor struct{ client probe.Getter }

func formatIGQualityLabel(width, height int, isVideo bool) string {
	if width <= 0 || height <= 0 {
		return "original"
	}

	if !isVideo {
		return fmt.Sprintf("%dx%d", width, height)
	}

	label := fmt.Sprintf("%dx%d/%dp", width, height, height)
	if height >= 720 {
		return fmt.Sprintf("%s (HD)", label)
	}
	if height >= 480 {
		return fmt.Sprintf("%s (SD)", label)
	}
	return label
}

// NewExtractor creates a new Instagram extractor with the given HTTP client.
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

// Match returns true if the URL is a valid Instagram URL.
func Match(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	h := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return h == "instagram.com" || h == "www.instagram.com" || h == "instagr.am"
}
func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

// Extract extracts media metadata from an Instagram URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	shortcode := extractShortcode(rawURL)
	if shortcode == "" {
		return nil, extract.Wrap(extract.KindInvalidInput, extract.ErrMsgInvalidURL, nil)
	}
	variables := fmt.Sprintf(`{"shortcode":"%s","fetch_tagged_user_count":null,"hoisted_comment_id":null,"hoisted_reply_id":null}`, shortcode)
	apiURL := fmt.Sprintf("%s?doc_id=%s&variables=%s", graphQLAPI, graphQLDocID, url.QueryEscape(variables))
	headers := map[string]string{"X-IG-App-ID": "936619743392459", "User-Agent": defaultUserAgent}
	if opts.UseAuth && opts.CookieHeader != "" {
		headers["Cookie"] = opts.CookieHeader
	}
	resp, err := e.client.Get(ctx, apiURL, headers)
	if err != nil {
		return nil, extract.Wrap(extract.KindUpstreamFailure, extract.ErrMsgUpstreamFailure, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, extract.WrapCode(extract.KindAuthRequired, "instagram_auth_required", "instagram requires authentication for this content", false, nil)
		case http.StatusNotFound:
			return nil, extract.WrapCode(extract.KindExtractionFailed, "instagram_media_not_found", "instagram media is unavailable or no longer exists", false, nil)
		case http.StatusTooManyRequests:
			return nil, extract.WrapCode(extract.KindUpstreamFailure, "instagram_rate_limited", "instagram temporarily rate limited the request", true, nil)
		default:
			return nil, extract.WrapCode(extract.KindUpstreamFailure, "instagram_upstream_status", fmt.Sprintf("instagram upstream returned status %d", resp.StatusCode), resp.StatusCode >= http.StatusInternalServerError, nil)
		}
	}
	var payload struct {
		Data struct {
			Media struct {
				ID         string `json:"id"`
				Typename   string `json:"__typename"`
				DisplayURL string `json:"display_url"`
				VideoURL   string `json:"video_url"`
				IsVideo    bool   `json:"is_video"`
				Dimensions struct {
					Width  int `json:"width"`
					Height int `json:"height"`
				} `json:"dimensions"`
				VideoViewCount int64 `json:"video_view_count"`
				Caption        struct {
					Edges []struct {
						Node struct {
							Text string `json:"text"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"edge_media_to_caption"`
				PreviewLike struct {
					Count int64 `json:"count"`
				} `json:"edge_media_preview_like"`
				CommentCount struct {
					Count int64 `json:"count"`
				} `json:"edge_media_to_comment"`
				Owner struct {
					Username string `json:"username"`
					FullName string `json:"full_name"`
				} `json:"owner"`
				Sidecar struct {
					Edges []struct {
						Node struct {
							Typename   string `json:"__typename"`
							DisplayURL string `json:"display_url"`
							VideoURL   string `json:"video_url"`
							IsVideo    bool   `json:"is_video"`
							Dimensions struct {
								Width  int `json:"width"`
								Height int `json:"height"`
							} `json:"dimensions"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"edge_sidecar_to_children"`
			} `json:"xdt_shortcode_media"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "invalid instagram payload", err)
	}
	m := payload.Data.Media
	if m.ID == "" {
		return nil, extract.Wrap(extract.KindExtractionFailed, "instagram media not found or private", nil)
	}
	caption := ""
	if len(m.Caption.Edges) > 0 {
		caption = strings.TrimSpace(m.Caption.Edges[0].Node.Text)
	}
	items := make([]extract.MediaItem, 0)
	appendNode := func(index int, isVideo bool, displayURL, videoURL string, width, height int) {
		itemType := "image"
		container := "jpg"
		mimeType := "image/jpeg"
		sourceURL := strings.TrimSpace(displayURL)
		hasAudio, hasVideo, progressive := false, false, false
		qualityLabel := "original"
		if isVideo {
			itemType = "video"
			container = "mp4"
			mimeType = "video/mp4"
			sourceURL = strings.TrimSpace(videoURL)
			hasAudio, hasVideo, progressive = true, true, true
			qualityLabel = formatIGQualityLabel(width, height, true)
		} else {
			qualityLabel = formatIGQualityLabel(width, height, false)
		}
		size := probe.SizeWithGetter(ctx, e.client, sourceURL, map[string]string{"User-Agent": defaultUserAgent, "Referer": rawURL})
		items = append(items, extract.MediaItem{Index: index, Type: itemType, ThumbnailURL: strings.TrimSpace(displayURL), FileSizeBytes: size, Sources: []extract.MediaSource{{Quality: qualityLabel, URL: sourceURL, Referer: rawURL, Origin: extract.SourceOrigin(rawURL), MIMEType: mimeType, Protocol: "https", Container: container, FileSizeBytes: size, Width: width, Height: height, HasAudio: hasAudio, HasVideo: hasVideo, IsProgressive: progressive}}})
	}
	if strings.Contains(strings.ToLower(m.Typename), "sidecar") {
		for i, edge := range m.Sidecar.Edges {
			appendNode(i, edge.Node.IsVideo, edge.Node.DisplayURL, edge.Node.VideoURL, edge.Node.Dimensions.Width, edge.Node.Dimensions.Height)
		}
	} else {
		appendNode(0, m.IsVideo, m.DisplayURL, m.VideoURL, m.Dimensions.Width, m.Dimensions.Height)
	}
	return extract.NewResultBuilder(rawURL, "instagram", "native").
		ContentType(detectContentType(rawURL, items)).
		Title(caption).
		Author(strings.TrimSpace(m.Owner.FullName), strings.TrimSpace(m.Owner.Username)).
		Engagement(m.VideoViewCount, m.PreviewLike.Count, m.CommentCount.Count, 0, 0).
		Media(items).
		Build(), nil
}

func extractShortcode(rawURL string) string {
	m := shortcodeRegex.FindStringSubmatch(rawURL)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
func detectContentType(rawURL string, items []extract.MediaItem) string {
	if strings.Contains(rawURL, "/reel/") || strings.Contains(rawURL, "/reels/") || (len(items) == 1 && items[0].Type == "video") {
		return "video"
	}
	return "post"
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0 Safari/537.36"
