package ariaextended

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
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
	baseCtx := opts.Ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
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

	return e.buildResultFromMeta(urlStr, meta, opts), nil
}

func (e *PythonExtractor) buildResultFromMeta(urlStr string, meta *core.YTDLPDumpJSON, opts core.ExtractOptions) *core.ExtractResult {
	if meta == nil {
		meta = &core.YTDLPDumpJSON{}
	}

	qualityForFormat := func(f core.YTDLPFormat) string {
		// Priority 1: Use height for video resolution (1080p, 720p, etc.)
		if f.Height > 0 {
			return fmt.Sprintf("%dp", f.Height)
		}
		// Priority 2: Use format note if available
		if note := strings.TrimSpace(f.FormatNote); note != "" {
			return note
		}
		// Priority 3: Use resolution string if available
		if resolution := strings.TrimSpace(f.Resolution); resolution != "" && resolution != "audio only" {
			return resolution
		}
		// Priority 4: For audio, use bitrate
		if f.ABR > 0 {
			return fmt.Sprintf("%dkbps", int(math.Round(f.ABR)))
		}
		if f.TBR > 0 {
			return fmt.Sprintf("%dkbps", int(math.Round(f.TBR)))
		}
		// Priority 5: Fallback to format ID
		if id := strings.TrimSpace(f.FormatID); id != "" {
			return id
		}
		// Priority 6: Fallback to extension
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

	// Deduplicate YouTube formats by resolution height
	// Keep only the highest bitrate format for each resolution
	videoByResolution := make(map[int]core.YTDLPFormat)
	fallbackVideoFormats := make(map[string]core.YTDLPFormat)
	var audioFormats []core.YTDLPFormat

	for _, f := range meta.Formats {
		if f.URL == "" || (f.VCodec == "none" && f.ACodec == "none") {
			continue
		}

		// Track audio-only formats separately
		if f.VCodec == "none" && f.ACodec != "none" {
			audioFormats = append(audioFormats, f)
			continue
		}

		// For video formats with known height, deduplicate by resolution height.
		if f.Height > 0 {
			existing, exists := videoByResolution[f.Height]
			if !exists {
				videoByResolution[f.Height] = f
				continue
			}

			existingHasKnownSize := existing.Filesize > 0 || existing.FilesizeApprox > 0
			newHasKnownSize := f.Filesize > 0 || f.FilesizeApprox > 0

			if !existingHasKnownSize && newHasKnownSize {
				videoByResolution[f.Height] = f
				continue
			}

			if existingHasKnownSize == newHasKnownSize && f.TBR > existing.TBR {
				videoByResolution[f.Height] = f
			}
			continue
		}

		// Some generic extractors expose quality but no height (e.g., 360/480/720/1080).
		// Keep one variant per logical quality key instead of dropping them.
		fallbackKey := ""
		if f.Quality > 0 {
			fallbackKey = fmt.Sprintf("%.0f", f.Quality)
		}
		if fallbackKey == "" {
			fallbackKey = strings.TrimSpace(f.FormatID)
		}
		if fallbackKey == "" {
			fallbackKey = strings.TrimSpace(f.URL)
		}
		existing, exists := fallbackVideoFormats[fallbackKey]
		if !exists || rankVideoFormat(f) > rankVideoFormat(existing) {
			fallbackVideoFormats[fallbackKey] = f
		}
	}

	// Rebuild sources: unique video formats + all audio formats
	videoFormats := make([]core.YTDLPFormat, 0, len(videoByResolution)+len(fallbackVideoFormats))
	for _, f := range videoByResolution {
		videoFormats = append(videoFormats, f)
	}
	for _, f := range fallbackVideoFormats {
		videoFormats = append(videoFormats, f)
	}

	// Highest quality first for generic/plugin outputs.
	sort.Slice(videoFormats, func(i, j int) bool {
		return rankVideoFormat(videoFormats[i]) > rankVideoFormat(videoFormats[j])
	})

	sort.Slice(audioFormats, func(i, j int) bool {
		if audioFormats[i].ABR != audioFormats[j].ABR {
			return audioFormats[i].ABR > audioFormats[j].ABR
		}
		return audioFormats[i].TBR > audioFormats[j].TBR
	})

	sources := make([]core.YTDLPFormat, 0, len(videoFormats)+len(audioFormats))
	sources = append(sources, videoFormats...)
	sources = append(sources, audioFormats...)

	if len(sources) == 0 && meta.URL != "" {
		sources = append(sources, core.YTDLPFormat{
			FormatID: "default",
			URL:      meta.URL,
			Ext:      meta.Ext,
		})
	}

	builder := core.NewResponseBuilder(urlStr).
		WithPlatform(e.resolvePlatform(meta, urlStr)).
		WithAuthor(meta.Uploader, meta.UploaderID).
		WithContent(meta.ID, meta.Title, meta.Description).
		WithEngagement(maxInt64(meta.ViewCount, 0), maxInt64(meta.LikeCount, 0), maxInt64(meta.CommentCount, 0), 0).
		WithAuthentication(opts.Cookie != "", opts.Source)

	variantTypes := make([]core.MediaType, 0, len(sources))
	variants := make([]core.Variant, 0, len(sources))

	for _, f := range sources {
		classification := core.ClassifyMedia(f.MimeType, f.Ext, f.VCodec, f.ACodec)
		variant := core.NewVariant(qualityForFormat(f), f.URL)
		if f.Resolution != "" && f.Resolution != "audio only" {
			variant = variant.WithResolution(f.Resolution)
		}
		if classification.Extension != "" {
			variant = variant.WithFormat(classification.Extension)
		}
		if classification.Mime != "" {
			variant = variant.WithMime(classification.Mime)
		}
		variant = variant.WithCodec(codecForFormat(f))
		if f.Filesize > 0 {
			variant = variant.WithFilesize(f.Filesize)
		} else if f.FilesizeApprox > 0 {
			variant = variant.WithFilesize(int64(math.Round(f.FilesizeApprox)))
		}
		variant = variant.WithAudio(f.ACodec != "" && f.ACodec != "none")
		variant = variant.WithMerge(f.VCodec != "" && f.VCodec != "none" && (f.ACodec == "" || f.ACodec == "none"))
		variant = variant.WithFormatID(f.FormatID)

		// Generate filename: author_title_id_[DownAria].ext
		extension := classification.Extension
		if extension == "" {
			extension = core.ClassifyMedia("", meta.Ext, "", "").Extension
		}
		if extension == "" {
			extension = "bin"
		}
		filename := core.GenerateFilenameWithMeta(meta.Uploader, meta.Title, meta.UploaderID, meta.ID, extension)
		variant = variant.WithFilename(filename)

		variants = append(variants, variant)
		variantTypes = append(variantTypes, classification.MediaType)
	}

	itemType := core.AggregateMediaTypes(variantTypes)
	if itemType == core.MediaTypeUnknown {
		itemType = core.ClassifyMedia("", meta.Ext, "", "").MediaType
	}
	if itemType == core.MediaTypeUnknown {
		itemType = core.MediaTypeVideo
	}

	builder = builder.WithMediaType(itemType)

	media := core.NewMedia(0, itemType, meta.Thumbnail)
	for _, variant := range variants {
		core.AddVariant(&media, variant)
	}

	builder.AddMedia(media)

	return builder.Build()
}

func rankVideoFormat(f core.YTDLPFormat) float64 {
	if f.Height > 0 {
		return float64(f.Height)*100000 + f.TBR
	}
	if f.Quality > 0 {
		return f.Quality*100000 + f.TBR
	}
	if fid := strings.TrimSpace(f.FormatID); fid != "" {
		if n, err := strconv.ParseFloat(fid, 64); err == nil {
			return n*100000 + f.TBR
		}
	}
	return f.TBR
}

var nonPlatformChars = regexp.MustCompile(`[^a-z0-9_-]+`)

func (e *PythonExtractor) resolvePlatform(meta *core.YTDLPDumpJSON, targetURL string) string {
	if static := normalizePlatformName(e.platform); static != "" {
		return static
	}

	if meta != nil {
		if fromExtractor := normalizePlatformName(meta.Extractor); fromExtractor != "" {
			return fromExtractor
		}
		if fromWebpage := platformFromURL(meta.WebpageURL); fromWebpage != "" {
			return fromWebpage
		}
	}

	if fromTarget := platformFromURL(targetURL); fromTarget != "" {
		return fromTarget
	}

	return "generic"
}

func normalizePlatformName(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	if idx := strings.IndexAny(value, ":/ "); idx > 0 {
		value = value[:idx]
	}
	value = strings.TrimSpace(nonPlatformChars.ReplaceAllString(value, ""))
	if value == "" {
		return ""
	}
	return value
}

func platformFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return ""
	}
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return normalizePlatformName(parts[len(parts)-2])
	}
	return normalizePlatformName(host)
}

func maxInt64(value, fallback int64) int64 {
	if value < 0 {
		return fallback
	}
	return value
}
