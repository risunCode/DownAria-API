package extraction

import (
	"context"
	"errors"
	"log"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	apperrors "fetchmoona/internal/core/errors"
	ariaextended "fetchmoona/internal/extractors/aria-extended"
	"fetchmoona/internal/extractors/core"
	"fetchmoona/internal/extractors/registry"
	"fetchmoona/internal/shared/security"
)

var (
	ErrInvalidURL          = errors.New("invalid url")
	ErrUnsupportedPlatform = errors.New("unsupported platform")
)

type ExtractInput struct {
	URL    string
	Cookie string
}

type Service interface {
	Extract(ctx context.Context, input ExtractInput) (*core.ExtractResult, error)
}

type extractionService struct {
	registry       *registry.Registry
	fallback       func() core.Extractor
	timeoutSeconds int
	maxRetries     int
	retryDelayMs   int
	serverCookies  map[string]string
}

type ServiceOption func(*extractionService)

func WithServerCookies(cookies map[string]string) ServiceOption {
	return func(s *extractionService) {
		if len(cookies) == 0 {
			s.serverCookies = nil
			return
		}
		normalized := make(map[string]string, len(cookies))
		for key, value := range cookies {
			platform := strings.ToLower(strings.TrimSpace(key))
			cookie := strings.TrimSpace(value)
			if platform == "" || cookie == "" {
				continue
			}
			normalized[platform] = cookie
		}
		if len(normalized) == 0 {
			s.serverCookies = nil
			return
		}
		s.serverCookies = normalized
	}
}

func WithFallbackExtractorFactory(factory func() core.Extractor) ServiceOption {
	return func(s *extractionService) {
		if factory == nil {
			return
		}
		s.fallback = factory
	}
}

func NewService(reg *registry.Registry, timeoutSeconds int, maxRetries int, retryDelayMs int, options ...ServiceOption) Service {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if retryDelayMs < 0 {
		retryDelayMs = 0
	}
	svc := &extractionService{
		registry:       reg,
		fallback:       func() core.Extractor { return ariaextended.NewPythonExtractor("") },
		timeoutSeconds: timeoutSeconds,
		maxRetries:     maxRetries,
		retryDelayMs:   retryDelayMs,
	}

	for _, opt := range options {
		if opt != nil {
			opt(svc)
		}
	}

	return svc
}

func (s *extractionService) Extract(ctx context.Context, input ExtractInput) (*core.ExtractResult, error) {
	targetURL, err := validateHTTPURL(input.URL)
	if err != nil {
		return nil, typedError{kind: ErrInvalidURL, err: err}
	}

	extractor, platform, err := s.registry.GetExtractor(targetURL)
	allowServerCookie := true
	if err != nil {
		if isNativePlatformURL(targetURL) {
			return nil, typedError{kind: ErrUnsupportedPlatform, err: err}
		}
		if s.fallback == nil {
			return nil, typedError{kind: ErrUnsupportedPlatform, err: err}
		}
		extractor = s.fallback()
		if extractor == nil {
			return nil, typedError{kind: ErrUnsupportedPlatform, err: err}
		}
		platform = ""
		allowServerCookie = false
	}

	userCookie := strings.TrimSpace(input.Cookie)
	baseOpts := core.ExtractOptions{
		Ctx:     ctx,
		Headers: nil,
		Timeout: s.timeoutSeconds,
		Source:  core.AuthSourceNone,
	}

	lanes := []core.ExtractOptions{baseOpts}
	if allowServerCookie {
		if serverCookie := s.resolveServerCookie(platform); serverCookie != "" {
			lane := baseOpts
			lane.Cookie = serverCookie
			lane.Source = core.AuthSourceServer
			lanes = append(lanes, lane)
		}
	}
	if userCookie != "" {
		lane := baseOpts
		lane.Cookie = userCookie
		lane.Source = core.AuthSourceClient
		lanes = append(lanes, lane)
	}

	var lastErr error
	for laneIndex, lane := range lanes {
		result, err := s.extractWithRetry(ctx, extractor, targetURL, lane, platform)
		if err == nil {
			if result != nil && result.Platform == "" {
				if platform != "" {
					result.Platform = platform
				} else {
					result.Platform = fallbackPlatformFromURL(targetURL)
				}
			}
			return result, nil
		}
		lastErr = err

		if laneIndex >= len(lanes)-1 || !shouldAdvanceAuthLane(err) {
			return nil, err
		}
	}

	return nil, lastErr
}

func (s *extractionService) extractWithRetry(ctx context.Context, extractor core.Extractor, targetURL string, opts core.ExtractOptions, platform string) (*core.ExtractResult, error) {
	var lastErr error
	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		result, err := s.callExtractor(extractor, targetURL, opts)
		if err == nil {
			return result, nil
		}

		lastErr = err
		categorized := apperrors.CategorizeError(err)
		if categorized == nil || !categorized.IsRetryable() {
			return nil, err
		}

		if attempt >= s.maxRetries {
			if categorized.Metadata == nil {
				categorized.Metadata = map[string]any{}
			}
			categorized.Metadata["attempts"] = attempt
			categorized.Metadata["lastError"] = err.Error()
			return nil, categorized
		}

		delay := time.Duration(math.Min(float64(s.retryDelayMs)*math.Pow(2, float64(attempt-1)), 30000)) * time.Millisecond
		log.Printf("[extraction] retry attempt=%d platform=%s source=%s delay=%s err=%s", attempt, platform, opts.Source, delay, security.RedactLogError(err))

		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

func shouldAdvanceAuthLane(err error) bool {
	if err == nil {
		return false
	}
	categorized := apperrors.CategorizeError(err)
	if categorized != nil {
		if categorized.Category == apperrors.CategoryAuth {
			return true
		}
		if categorized.Code == apperrors.CodeNoMediaFound {
			return true
		}
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "no media") || strings.Contains(msg, "media not found") {
		return true
	}

	authHints := []string{
		"login required",
		"authentication required",
		"auth required",
		"requires cookie",
		"content is private",
	}
	for _, hint := range authHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}

	return false
}

func (s *extractionService) resolveServerCookie(platform string) string {
	if len(s.serverCookies) == 0 {
		return ""
	}
	return strings.TrimSpace(s.serverCookies[strings.ToLower(strings.TrimSpace(platform))])
}

func (s *extractionService) callExtractor(extractor core.Extractor, targetURL string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	return extractor.Extract(targetURL, opts)
}

func fallbackPlatformFromURL(targetURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return "generic"
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "generic"
	}
	host = strings.TrimPrefix(host, "www.")
	parts := strings.Split(host, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return host
}

func isNativePlatformURL(targetURL string) bool {
	nativePatterns := [][]*regexp.Regexp{
		registry.FacebookPatterns,
		registry.InstagramPatterns,
		registry.ThreadsPatterns,
		registry.TwitterPatterns,
		registry.TikTokPatterns,
		registry.PixivPatterns,
	}

	for _, patterns := range nativePatterns {
		for _, pattern := range patterns {
			if pattern.MatchString(targetURL) {
				return true
			}
		}
	}

	return false
}

func validateHTTPURL(raw string) (string, error) {
	return security.SanitizeHTTPURLString(raw)
}

type typedError struct {
	kind error
	err  error
}

func (e typedError) Error() string {
	return e.err.Error()
}

func (e typedError) Unwrap() error {
	return e.kind
}
