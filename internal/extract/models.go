package extract

import (
	"context"
	"log/slog"
	"net/url"
)

type Kind string

type StreamProfile string

const (
	StreamProfileImage             StreamProfile = "image"
	StreamProfileAudioOnly         StreamProfile = "audio_only"
	StreamProfileVideoOnlyAdaptive StreamProfile = "video_only_adaptive"
	StreamProfileVideoOnlyProg     StreamProfile = "video_only_progressive"
	StreamProfileMuxedProg         StreamProfile = "muxed_progressive"
	StreamProfileMuxedAdaptive     StreamProfile = "muxed_adaptive"
)

const (
	KindInvalidInput        Kind = "invalid_input"
	KindUnsupportedPlatform Kind = "unsupported_platform"
	KindAuthRequired        Kind = "auth_required"
	KindUpstreamFailure     Kind = "upstream_failure"
	KindExtractionFailed    Kind = "extraction_failed"
	KindDownloadFailed      Kind = "download_failed"
	KindMergeFailed         Kind = "merge_failed"
	KindConvertFailed       Kind = "convert_failed"
	KindTimeout             Kind = "timeout"
	KindInternal            Kind = "internal"
	KindRateLimited         Kind = "rate_limited"
)

type AppError struct {
	Kind      Kind
	Code      string
	Message   string
	Retryable bool
	Err       error
}

var (
	ErrInvalidInput        = &AppError{Kind: KindInvalidInput}
	ErrUnsupportedPlatform = &AppError{Kind: KindUnsupportedPlatform}
	ErrAuthRequired        = &AppError{Kind: KindAuthRequired}
	ErrUpstreamFailure     = &AppError{Kind: KindUpstreamFailure}
	ErrExtractionFailed    = &AppError{Kind: KindExtractionFailed}
	ErrDownloadFailed      = &AppError{Kind: KindDownloadFailed}
	ErrMergeFailed         = &AppError{Kind: KindMergeFailed}
	ErrConvertFailed       = &AppError{Kind: KindConvertFailed}
	ErrTimeout             = &AppError{Kind: KindTimeout}
	ErrInternal            = &AppError{Kind: KindInternal}
)

type Result struct {
	SourceURL      string      `json:"source_url"`
	Platform       string      `json:"platform"`
	ExtractProfile string      `json:"extract_profile,omitempty"`
	ContentType    string      `json:"content_type"`
	Visibility     string      `json:"visibility,omitempty"`
	Title          string      `json:"title,omitempty"`
	Author         Author      `json:"author,omitempty"`
	Filename       string      `json:"filename,omitempty"`
	Engagement     Engagement  `json:"engagement,omitempty"`
	FileSizeBytes  int64       `json:"file_size_bytes,omitempty"`
	Media          []MediaItem `json:"media"`
}

type Author struct {
	Name   string `json:"name,omitempty"`
	Handle string `json:"handle,omitempty"`
}

type Engagement struct {
	Views     int64 `json:"views,omitempty"`
	Likes     int64 `json:"likes,omitempty"`
	Comments  int64 `json:"comments,omitempty"`
	Shares    int64 `json:"shares,omitempty"`
	Bookmarks int64 `json:"bookmarks,omitempty"`
}

type MediaItem struct {
	Index         int           `json:"index"`
	Type          string        `json:"type"`
	Filename      string        `json:"filename,omitempty"`
	ThumbnailURL  string        `json:"thumbnail_url,omitempty"`
	FileSizeBytes int64         `json:"file_size_bytes,omitempty"`
	Sources       []MediaSource `json:"sources"`
}

type MediaSource struct {
	FormatID        string        `json:"format_id,omitempty"`
	Quality         string        `json:"quality,omitempty"`
	URL             string        `json:"url"`
	Referer         string        `json:"referer,omitempty"`
	Origin          string        `json:"origin,omitempty"`
	MIMEType        string        `json:"mime_type,omitempty"`
	Protocol        string        `json:"protocol,omitempty"`
	Container       string        `json:"container,omitempty"`
	VideoCodec      string        `json:"video_codec,omitempty"`
	AudioCodec      string        `json:"audio_codec,omitempty"`
	Width           int           `json:"width,omitempty"`
	Height          int           `json:"height,omitempty"`
	DurationSeconds float64       `json:"duration_seconds,omitempty"`
	FileSizeBytes   int64         `json:"file_size_bytes,omitempty"`
	StreamProfile   StreamProfile `json:"stream_profile,omitempty"`
	HasAudio        bool          `json:"has_audio"`
	HasVideo        bool          `json:"has_video"`
	IsProgressive   bool          `json:"is_progressive"`
	NeedsProxy      bool          `json:"needs_proxy,omitempty"`
}

type ExtractOptions struct {
	CookieHeader string
	UseAuth      bool
}

type URLValidator interface {
	Validate(ctx context.Context, rawURL string) (*url.URL, error)
}

type Extractor interface {
	Match(rawURL string) bool
	Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error)
}

type UniversalExtractor interface {
	Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error)
}

type Service interface {
	Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error)
}

type Entry struct {
	Platform  string
	Extractor Extractor
}

type Registry struct{ entries []Entry }

type service struct {
	registry  *Registry
	universal UniversalExtractor
	validator URLValidator
	cache     Cache
	logger    *slog.Logger
}

// ResultBuilder provides a fluent interface for building extract.Result consistently
// across all platform extractors (native and generic/ytdlp).
//
// Usage:
//
//	result := extract.NewResultBuilder(sourceURL, "twitter", "native").
//		Title("Post title").
//		Author("John Doe", "@johndoe").
//		Engagement(views, likes, comments, shares, bookmarks).
//		Media(mediaItems).
//		Build()
//
// This ensures consistent JSON response structure across all extractors.
type ResultBuilder struct {
	result *Result
}

// NewResultBuilder creates a new ResultBuilder with required fields
func NewResultBuilder(sourceURL, platform, profile string) *ResultBuilder {
	return &ResultBuilder{
		result: &Result{
			SourceURL:      sourceURL,
			Platform:       platform,
			ExtractProfile: profile,
			ContentType:    "post",
			Visibility:     "public",
		},
	}
}

func (b *ResultBuilder) ContentType(ct string) *ResultBuilder {
	b.result.ContentType = ct
	return b
}

func (b *ResultBuilder) Visibility(v string) *ResultBuilder {
	b.result.Visibility = v
	return b
}

func (b *ResultBuilder) Title(title string) *ResultBuilder {
	b.result.Title = title
	return b
}

func (b *ResultBuilder) Author(name, handle string) *ResultBuilder {
	b.result.Author = Author{Name: name, Handle: handle}
	return b
}

func (b *ResultBuilder) Engagement(views, likes, comments, shares, bookmarks int64) *ResultBuilder {
	b.result.Engagement = Engagement{
		Views:     SanitizeStat(views),
		Likes:     SanitizeStat(likes),
		Comments:  SanitizeStat(comments),
		Shares:    SanitizeStat(shares),
		Bookmarks: SanitizeStat(bookmarks),
	}
	return b
}

func (b *ResultBuilder) Media(media []MediaItem) *ResultBuilder {
	b.result.Media = media
	b.result.FileSizeBytes = SumMediaSizes(media)
	return b
}

func (b *ResultBuilder) Build() *Result {
	return b.result
}
