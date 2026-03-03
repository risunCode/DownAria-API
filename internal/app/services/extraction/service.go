package extraction

import (
	"context"
	"errors"
	"log"
	"math"
	"net/url"
	"strings"
	"time"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/extractors/core"
	"downaria-api/internal/extractors/registry"
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

func NewService(reg *registry.Registry, timeoutSeconds int, maxRetries int, retryDelayMs int, options ...ServiceOption) Service {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if retryDelayMs < 0 {
		retryDelayMs = 0
	}
	svc := &extractionService{
		registry:       reg,
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
	if err != nil {
		return nil, typedError{kind: ErrUnsupportedPlatform, err: err}
	}

	userCookie := strings.TrimSpace(input.Cookie)
	baseOpts := core.ExtractOptions{
		Ctx:     ctx,
		Headers: nil,
		Timeout: s.timeoutSeconds,
		Source:  core.AuthSourceNone,
	}

	lanes := []core.ExtractOptions{baseOpts}
	if serverCookie := s.resolveServerCookie(platform); serverCookie != "" {
		lane := baseOpts
		lane.Cookie = serverCookie
		lane.Source = core.AuthSourceServer
		lanes = append(lanes, lane)
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
				result.Platform = platform
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
		log.Printf("[extraction] retry attempt=%d platform=%s source=%s delay=%s err=%v", attempt, platform, opts.Source, delay, err)

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
	if categorized == nil {
		return false
	}
	return categorized.Category == apperrors.CategoryAuth
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

func validateHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("unsupported scheme")
	}
	if parsed.Host == "" {
		return "", errors.New("missing host")
	}
	return parsed.String(), nil
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
