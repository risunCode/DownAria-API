package instagram

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
)

const (
	GraphQLAPI   = "https://www.instagram.com/graphql/query"
	GraphQLDocID = "8845758582119845"
)

var shortcodeRegex = regexp.MustCompile(`/(?:p|reels?|tv)/([A-Za-z0-9_-]+)`)

type InstagramExtractor struct {
	*core.BaseExtractor
}

func NewInstagramExtractor() *InstagramExtractor {
	return &InstagramExtractor{
		BaseExtractor: core.NewBaseExtractor(),
	}
}

func (e *InstagramExtractor) Match(urlStr string) bool {
	return e.MatchHost(urlStr, []string{"instagram.com", "www.instagram.com", "instagr.am"})
}

func (e *InstagramExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	// 1. Extract Shortcode
	shortcode := e.extractShortcode(urlStr)
	if shortcode == "" {
		return nil, fmt.Errorf("invalid instagram URL: shortcode not found")
	}

	// 2. Fetch via GraphQL API (Public)
	variables := fmt.Sprintf(`{"shortcode":"%s","fetch_tagged_user_count":null,"hoisted_comment_id":null,"hoisted_reply_id":null}`, shortcode)
	apiURL := fmt.Sprintf("%s?doc_id=%s&variables=%s", GraphQLAPI, GraphQLDocID, url.QueryEscape(variables))

	resp, err := e.MakeRequest("GET", apiURL, nil, opts, map[string]string{"X-IG-App-ID": "936619743392459"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := e.CheckStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}

	// 3. Parse Response
	var data struct {
		Data struct {
			XDTShortcodeMedia struct {
				ID             string `json:"id"`
				Shortcode      string `json:"shortcode"`
				Typename       string `json:"__typename"`
				DisplayURL     string `json:"display_url"`
				VideoURL       string `json:"video_url"`
				IsVideo        bool   `json:"is_video"`
				VideoViewCount int64  `json:"video_view_count"`
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
				EdgeSidecar struct {
					Edges []struct {
						Node struct {
							Typename   string `json:"__typename"`
							DisplayURL string `json:"display_url"`
							VideoURL   string `json:"video_url"`
							IsVideo    bool   `json:"is_video"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"edge_sidecar_to_children"`
			} `json:"xdt_shortcode_media"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	mediaData := data.Data.XDTShortcodeMedia
	if mediaData.ID == "" {
		return nil, fmt.Errorf("media not found or private")
	}

	caption := ""
	if len(mediaData.Caption.Edges) > 0 {
		caption = strings.TrimSpace(mediaData.Caption.Edges[0].Node.Text)
	}

	// 4. Build Result using ResponseBuilder
	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("instagram").
		WithMediaType(e.detectMediaType(urlStr)).
		WithAuthor(mediaData.Owner.FullName, mediaData.Owner.Username).
		WithContent(mediaData.ID, caption, caption).
		WithEngagement(
			util.ClampNonNegativeInt64(mediaData.VideoViewCount),
			util.ClampNonNegativeInt64(mediaData.PreviewLike.Count),
			util.ClampNonNegativeInt64(mediaData.CommentCount.Count),
			0,
		).
		WithAuthentication(opts.Cookie != "", opts.Source)

	// Handle carousel
	if mediaData.Typename == "XDTGraphSidecar" || mediaData.Typename == "GraphSidecar" {
		for i, edge := range mediaData.EdgeSidecar.Edges {
			node := edge.Node
			media := core.NewMedia(i, core.MediaTypeImage, node.DisplayURL)
			if node.IsVideo {
				media.Type = core.MediaTypeVideo
				variant := core.NewVideoVariant("HD", node.VideoURL)
				filename := core.GenerateFilenameWithMeta(
					mediaData.Owner.Username,
					caption,
					mediaData.Owner.Username,
					mediaData.ID,
					"mp4",
				)
				variant = variant.WithFilename(filename)
				core.AddVariant(&media, variant)
			} else {
				variant := core.NewImageVariant("Original", node.DisplayURL)
				filename := core.GenerateFilenameWithMeta(
					mediaData.Owner.Username,
					caption,
					mediaData.Owner.Username,
					mediaData.ID,
					"jpg",
				)
				variant = variant.WithFilename(filename)
				core.AddVariant(&media, variant)
			}
			builder.AddMedia(media)
		}
	} else {
		media := core.NewMedia(0, core.MediaTypeImage, mediaData.DisplayURL)
		if mediaData.IsVideo {
			media.Type = core.MediaTypeVideo
			variant := core.NewVideoVariant("HD", mediaData.VideoURL)
			filename := core.GenerateFilenameWithMeta(
				mediaData.Owner.Username,
				caption,
				mediaData.Owner.Username,
				mediaData.ID,
				"mp4",
			)
			variant = variant.WithFilename(filename)
			core.AddVariant(&media, variant)
		} else {
			variant := core.NewImageVariant("Original", mediaData.DisplayURL)
			filename := core.GenerateFilenameWithMeta(
				mediaData.Owner.Username,
				caption,
				mediaData.Owner.Username,
				mediaData.ID,
				"jpg",
			)
			variant = variant.WithFilename(filename)
			core.AddVariant(&media, variant)
		}
		builder.AddMedia(media)
	}

	return builder.Build(), nil
}

func (e *InstagramExtractor) extractShortcode(urlStr string) string {
	return util.ExtractFirstRegexGroup(urlStr, shortcodeRegex)
}

func (e *InstagramExtractor) detectMediaType(urlStr string) core.MediaType {
	if strings.Contains(urlStr, "/reel/") {
		return core.MediaTypeReel
	}
	if strings.Contains(urlStr, "/stories/") {
		return core.MediaTypeStory
	}
	return core.MediaTypePost
}
