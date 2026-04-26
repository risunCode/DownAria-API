package extract

import (
	"context"
	"downaria-api/internal/api/middleware"
	logging "downaria-api/internal/logging"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

func NewRegistry(entries ...Entry) *Registry {
	copyEntries := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Extractor != nil && entry.Platform != "" {
			copyEntries = append(copyEntries, entry)
		}
	}
	return &Registry{entries: copyEntries}
}

func (r *Registry) Resolve(rawURL string) (Extractor, string, error) {
	if r == nil {
		return nil, "", Wrap(KindInternal, "extractor registry is nil", nil)
	}
	for _, entry := range r.entries {
		if entry.Extractor.Match(rawURL) {
			return entry.Extractor, entry.Platform, nil
		}
	}
	return nil, "", nil
}

func NewService(registry *Registry, universal UniversalExtractor, validator URLValidator, cache Cache, loggers ...*slog.Logger) Service {
	return &service{registry: registry, universal: universal, validator: validator, cache: cache, logger: logging.FallbackLogger(loggers...)}
}

func (s *service) Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, Wrap(KindInvalidInput, "url is required", nil)
	}
	if s.validator != nil {
		if _, err := s.validator.Validate(ctx, rawURL); err != nil {
			return nil, Wrap(KindInvalidInput, "invalid url", err)
		}
	} else {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, Wrap(KindInvalidInput, "invalid url", err)
		}
	}
	var extractor Extractor
	var platform string
	var err error
	if s.registry != nil {
		extractor, platform, err = s.registry.Resolve(rawURL)
		if err != nil {
			return nil, err
		}
	}
	normalizedOpts := normalizeOptions(opts)
	cacheKey := CacheKey(rawURL, normalizedOpts)
	if s.cache != nil {
		cached, ok, err := s.cache.Get(cacheKey)
		if err == nil && ok {
			s.logInfo(ctx, "extract_stage", slog.String("stage", "cache_hit"), slog.String("url", rawURL), slog.String("platform", cached.Platform))
			return finalizeResult(cached, rawURL, cached.Platform, cached.ExtractProfile)
		}
	}
	if extractor != nil {
		s.logInfo(ctx, "extract_stage", slog.String("stage", "native_extract"), slog.String("url", rawURL), slog.String("platform", platform))
		result, err := extractor.Extract(ctx, rawURL, normalizedOpts)
		if err != nil {
			s.logWarn(ctx, "extract_stage", slog.String("stage", "native_extract_failed"), slog.String("url", rawURL), slog.String("platform", platform), slog.String("error", err.Error()))
			if appErr := AsAppError(err); appErr != nil {
				return nil, appErr
			}
			return nil, Wrap(KindExtractionFailed, "extraction failed", err)
		}
		final, err := finalizeResult(result, rawURL, platform, "native")
		if err == nil {
			applyVisibility(final, normalizedOpts)
		}
		if err == nil && s.cache != nil {
			_ = s.cache.Set(cacheKey, final)
		}
		if err == nil {
			s.logInfo(ctx, "extract_stage", slog.String("stage", "native_extract_completed"), slog.String("url", rawURL), slog.String("platform", final.Platform))
		}
		return final, err
	}
	if s.universal == nil {
		return nil, Wrap(KindUnsupportedPlatform, "platform is not supported", nil)
	}
	s.logInfo(ctx, "extract_stage", slog.String("stage", "universal_extract"), slog.String("url", rawURL), slog.String("platform", platform))
	result, err := s.universal.Extract(ctx, rawURL, normalizedOpts)
	if err != nil {
		s.logWarn(ctx, "extract_stage", slog.String("stage", "universal_extract_failed"), slog.String("url", rawURL), slog.String("error", err.Error()))
		if appErr := AsAppError(err); appErr != nil {
			return nil, appErr
		}
		return nil, Wrap(KindExtractionFailed, "extraction failed", err)
	}
	final, err := finalizeResult(result, rawURL, platform, "generic")
	if err == nil {
		applyVisibility(final, normalizedOpts)
	}
	if err == nil && s.cache != nil {
		_ = s.cache.Set(cacheKey, final)
	}
	if err == nil {
		s.logInfo(ctx, "extract_stage", slog.String("stage", "universal_extract_completed"), slog.String("url", rawURL), slog.String("platform", final.Platform))
	}
	return final, err
}

func applyVisibility(result *Result, opts ExtractOptions) {
	if result == nil {
		return
	}
	if opts.UseAuth && strings.TrimSpace(opts.CookieHeader) != "" {
		result.Visibility = "private"
		return
	}
	if strings.TrimSpace(result.Visibility) == "" || strings.EqualFold(strings.TrimSpace(result.Visibility), "unknown") {
		result.Visibility = "public"
	}
}

func finalizeResult(result *Result, rawURL, platform, extractProfile string) (*Result, error) {
	if result == nil {
		return nil, Wrap(KindExtractionFailed, "extractor returned no result", nil)
	}
	if result.Platform == "" {
		result.Platform = platform
	}
	normalizeFileSizes(result)
	if strings.TrimSpace(result.Visibility) == "" {
		result.Visibility = "unknown"
	}
	if strings.TrimSpace(result.ExtractProfile) == "" {
		result.ExtractProfile = strings.TrimSpace(extractProfile)
	}
	if result.SourceURL == "" {
		result.SourceURL = rawURL
	}
	if result.Media == nil {
		result.Media = []MediaItem{}
	}
	for i := range result.Media {
		for j := range result.Media[i].Sources {
			s := &result.Media[i].Sources[j]
			if s.StreamProfile == "" {
				s.StreamProfile = inferStreamProfile(result.Media[i].Type, s)
			}
		}
	}
	result.ContentType = normalizeContentType(result.ContentType, result.Media)
	applyFilename(result)
	return result, nil
}

func normalizeFileSizes(result *Result) {
	if result == nil {
		return
	}
	totalResultSize := int64(0)
	for i := range result.Media {
		itemTotal := int64(0)
		for j := range result.Media[i].Sources {
			size := normalizeSizeValue(result.Media[i].Sources[j].FileSizeBytes)
			result.Media[i].Sources[j].FileSizeBytes = size
			itemTotal += size
		}
		result.Media[i].FileSizeBytes = normalizeSizeValue(result.Media[i].FileSizeBytes)
		if itemTotal > 0 {
			result.Media[i].FileSizeBytes = itemTotal
		}
		totalResultSize += result.Media[i].FileSizeBytes
	}
	result.FileSizeBytes = normalizeSizeValue(result.FileSizeBytes)
	if totalResultSize > 0 {
		result.FileSizeBytes = totalResultSize
	}
}

func normalizeSizeValue(size int64) int64 {
	if size <= 1 {
		return 0
	}
	return size
}

func inferStreamProfile(itemType string, s *MediaSource) StreamProfile {
	if strings.ToLower(itemType) == "image" || strings.HasPrefix(strings.ToLower(s.MIMEType), "image/") {
		return StreamProfileImage
	}
	if s.HasVideo {
		if s.HasAudio {
			return StreamProfileMuxedProg
		}
		if s.IsProgressive {
			return StreamProfileVideoOnlyProg
		}
		return StreamProfileVideoOnlyAdaptive
	}
	if s.HasAudio {
		return StreamProfileAudioOnly
	}
	return ""
}

func FinalizeResult(result *Result, rawURL, platform, extractProfile string) (*Result, error) {
	return finalizeResult(result, rawURL, platform, extractProfile)
}
func normalizeOptions(opts ExtractOptions) ExtractOptions {
	opts.CookieHeader = strings.TrimSpace(opts.CookieHeader)
	opts.UseAuth = opts.UseAuth && opts.CookieHeader != ""
	return opts
}

func (s *service) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if s == nil || s.logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("request_id", middleware.FromContext(ctx))}, attrs...)
	s.logger.LogAttrs(ctx, level, message, attrs...)
}

func (s *service) logInfo(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelInfo, msg, attrs...)
}

func (s *service) logWarn(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelWarn, msg, attrs...)
}
