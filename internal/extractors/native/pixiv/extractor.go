package pixiv

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"fetchmoona/internal/extractors/core"
	"fetchmoona/internal/shared/util"
)

var artworkIDRegex = regexp.MustCompile(`artworks/(\d+)`)

type PixivExtractor struct {
	*core.BaseExtractor
}

func NewPixivExtractor() *PixivExtractor {
	return &PixivExtractor{
		BaseExtractor: core.NewBaseExtractor(),
	}
}

func (e *PixivExtractor) Match(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return strings.Contains(u.Host, "pixiv.net")
}

func (e *PixivExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	// 1. Extract Artwork ID
	id := e.extractArtworkID(urlStr)
	if id == "" {
		return nil, fmt.Errorf("invalid pixiv URL: artwork ID not found")
	}

	// 2. Fetch AJAX API (Public)
	apiURL := fmt.Sprintf("https://www.pixiv.net/ajax/illust/%s", id)
	resp, err := e.MakeRequest("GET", apiURL, nil, opts, map[string]string{"Referer": "https://www.pixiv.net/"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := e.CheckStatus(resp, http.StatusOK); err != nil {
		return nil, err
	}

	// 3. Parse Response
	var data struct {
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
			Urls          struct {
				Original string `json:"original"`
			} `json:"urls"`
			PageCount int `json:"pageCount"`
		} `json:"body"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.Error {
		return nil, fmt.Errorf("pixiv API error: %s", data.Message)
	}

	// 4. Build Result
	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("pixiv").
		WithMediaType(core.MediaTypePost).
		WithAuthor(data.Body.UserName, data.Body.UserAccount).
		WithContent(id, data.Body.IllustTitle, "").
		WithEngagement(
			util.ClampNonNegativeInt64(data.Body.ViewCount),
			util.ClampNonNegativeInt64(data.Body.LikeCount),
			util.ClampNonNegativeInt64(data.Body.CommentCount),
			0,
		).
		WithBookmarks(util.ClampNonNegativeInt64(data.Body.BookmarkCount)).
		WithAuthentication(opts.Cookie != "", opts.Source)

	originalURL := data.Body.Urls.Original
	if data.Body.PageCount > 1 {
		for i := 0; i < data.Body.PageCount; i++ {
			pageURL := strings.Replace(originalURL, "_p0", fmt.Sprintf("_p%d", i), 1)
			media := core.NewMedia(i, core.MediaTypeImage, "")
			variant := core.NewImageProxyVariant("Original", pageURL)
			filename := core.GenerateFilenameWithMeta(data.Body.UserName, data.Body.IllustTitle, data.Body.UserName, id, "jpg")
			variant = variant.WithFilename(filename)
			core.AddVariant(&media, variant)
			builder.AddMedia(media)
		}
	} else {
		media := core.NewMedia(0, core.MediaTypeImage, "")
		variant := core.NewImageProxyVariant("Original", originalURL)
		filename := core.GenerateFilenameWithMeta(data.Body.UserName, data.Body.IllustTitle, data.Body.UserName, id, "jpg")
		variant = variant.WithFilename(filename)
		core.AddVariant(&media, variant)
		builder.AddMedia(media)
	}

	return builder.Build(), nil
}

func (e *PixivExtractor) extractArtworkID(urlStr string) string {
	return util.ExtractFirstRegexGroup(urlStr, artworkIDRegex)
}
