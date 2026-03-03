package ariaextended

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"downaria-api/internal/extractors/core"
)

type PythonExtractor struct {
	platform string
}

func NewPythonExtractor(platform string) *PythonExtractor {
	return &PythonExtractor{platform: platform}
}

func (e *PythonExtractor) Match(url string) bool {
	return true
}

func (e *PythonExtractor) Extract(urlStr string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	extraArgs := make([]string, 0, 6)
	if cookie := strings.TrimSpace(opts.Cookie); cookie != "" {
		extraArgs = append(extraArgs, "--add-header", fmt.Sprintf("Cookie: %s", cookie))
	}
	for key, value := range opts.Headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		extraArgs = append(extraArgs, "--add-header", fmt.Sprintf("%s: %s", key, value))
	}

	meta, err := core.RunYtDlpDump(ctx, urlStr, extraArgs...)
	if err != nil {
		return nil, err
	}

	itemType := core.MediaTypeVideo
	hasVideo := false
	for _, format := range meta.Formats {
		if format.VCodec != "" && format.VCodec != "none" {
			hasVideo = true
			break
		}
	}
	if !hasVideo {
		itemType = core.MediaTypeAudio
	}

	qualityForFormat := func(f core.YTDLPFormat) string {
		if note := strings.TrimSpace(f.FormatNote); note != "" {
			return note
		}
		if f.Height > 0 {
			return fmt.Sprintf("%dp", f.Height)
		}
		if resolution := strings.TrimSpace(f.Resolution); resolution != "" && resolution != "audio only" {
			return resolution
		}
		if f.ABR > 0 {
			return fmt.Sprintf("%dkbps", int(math.Round(f.ABR)))
		}
		if f.TBR > 0 {
			return fmt.Sprintf("%dkbps", int(math.Round(f.TBR)))
		}
		if id := strings.TrimSpace(f.FormatID); id != "" {
			return id
		}
		if ext := strings.TrimSpace(f.Ext); ext != "" {
			return strings.ToUpper(ext)
		}
		return "default"
	}

	codecForFormat := func(f core.YTDLPFormat) string {
		if f.VCodec != "" && f.VCodec != "none" {
			return f.VCodec
		}
		if f.ACodec != "" && f.ACodec != "none" {
			return f.ACodec
		}
		return ""
	}

	sources := make([]core.YTDLPFormat, 0, len(meta.Formats))
	for _, f := range meta.Formats {
		if f.URL == "" || (f.VCodec == "none" && f.ACodec == "none") {
			continue
		}
		sources = append(sources, f)
	}

	if len(sources) == 0 && meta.URL != "" {
		sources = append(sources, core.YTDLPFormat{
			FormatID: "default",
			URL:      meta.URL,
			Ext:      meta.Ext,
		})
	}

	mediaType := core.MediaTypeVideo
	if !hasVideo {
		mediaType = core.MediaTypeAudio
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform(e.platform).
		WithMediaType(mediaType).
		WithAuthor(meta.Uploader, meta.UploaderID).
		WithContent(meta.ID, meta.Title, meta.Description).
		WithEngagement(maxInt64(meta.ViewCount, 0), maxInt64(meta.LikeCount, 0), maxInt64(meta.CommentCount, 0), 0).
		WithAuthentication(opts.Cookie != "", opts.Source)

	media := core.NewMedia(0, itemType, meta.Thumbnail)

	for _, f := range sources {
		variant := core.NewVariant(qualityForFormat(f), f.URL)
		if f.Resolution != "" && f.Resolution != "audio only" {
			variant = variant.WithResolution(f.Resolution)
		}
		if f.Ext != "" {
			variant = variant.WithFormat(f.Ext)
		}
		variant = variant.WithCodec(codecForFormat(f))
		if f.Filesize > 0 {
			variant = variant.WithSize(f.Filesize)
		} else if f.FilesizeApprox > 0 {
			variant = variant.WithSize(int64(math.Round(f.FilesizeApprox)))
		}
		variant = variant.WithAudio(f.ACodec != "" && f.ACodec != "none")
		variant = variant.WithMerge(f.VCodec != "" && f.VCodec != "none" && (f.ACodec == "" || f.ACodec == "none"))
		variant = variant.WithFormatID(f.FormatID)

		core.AddVariant(&media, variant)
	}

	builder.AddMedia(media)

	return builder.Build(), nil
}

func maxInt64(value, fallback int64) int64 {
	if value < 0 {
		return fallback
	}
	return value
}
