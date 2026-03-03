package tiktok

import (
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
)

const TikWMAPI = "https://www.tikwm.com/api/"

type TikTokExtractor struct {
	*core.BaseExtractor
}

func NewTikTokExtractor() *TikTokExtractor {
	return &TikTokExtractor{
		BaseExtractor: core.NewBaseExtractor(),
	}
}

func (e *TikTokExtractor) Match(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "tiktok.com")
}

func (e *TikTokExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	data := url.Values{}
	data.Set("url", urlStr)
	data.Set("hd", "1")

	if opts.Headers == nil {
		opts.Headers = map[string]string{}
	}
	opts.Headers["Content-Type"] = "application/x-www-form-urlencoded"
	opts.Headers["User-Agent"] = util.DefaultUserAgent

	resp, err := e.MakeRequest("POST", TikWMAPI, strings.NewReader(data.Encode()), opts, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var bodyReader io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gzReader.Close()
			bodyReader = gzReader
		}
	case "deflate":
		zReader, err := zlib.NewReader(resp.Body)
		if err == nil {
			defer zReader.Close()
			bodyReader = zReader
		}
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Duration     int    `json:"duration"`
			CreateTime   int64  `json:"create_time"`
			Play         string `json:"play"`
			WMPlay       string `json:"wmplay"`
			HDPlay       string `json:"hdplay"`
			PlayCount    int64  `json:"play_count"`
			DiggCount    int64  `json:"digg_count"`
			CommentCount int64  `json:"comment_count"`
			ShareCount   int64  `json:"share_count"`
			Author       struct {
				Nickname string `json:"nickname"`
				UniqueId string `json:"unique_id"`
			} `json:"author"`
			OriginCover string `json:"origin_cover"`
		} `json:"data"`
	}

	if err := json.NewDecoder(bodyReader).Decode(&result); err != nil {
		return nil, err
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("tiktok extraction failed: %s", result.Msg)
	}

	videoURL := result.Data.HDPlay
	if videoURL == "" {
		videoURL = result.Data.Play
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform("tiktok").
		WithMediaType(core.MediaTypeVideo).
		WithAuthor(result.Data.Author.Nickname, result.Data.Author.UniqueId).
		WithContent(result.Data.ID, result.Data.Title, "").
		WithEngagement(
			util.ClampNonNegativeInt64(result.Data.PlayCount),
			util.ClampNonNegativeInt64(result.Data.DiggCount),
			util.ClampNonNegativeInt64(result.Data.CommentCount),
			util.ClampNonNegativeInt64(result.Data.ShareCount),
		).
		WithAuthentication(opts.Cookie != "", opts.Source)

	media := core.NewMedia(0, core.MediaTypeVideo, result.Data.OriginCover)
	variant := core.NewVideoVariant("HD", videoURL)
	filename := core.GenerateFilename(result.Data.Author.Nickname, result.Data.Title, result.Data.ID, "mp4")
	variant = variant.WithFilename(filename)
	core.AddVariant(&media, variant)

	builder.AddMedia(media)

	return builder.Build(), nil
}
