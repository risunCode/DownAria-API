package extraction

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/extractors/core"
	"downaria-api/internal/extractors/registry"
	"downaria-api/internal/infra/cache"
)

// mockExtractor implements core.Extractor for testing
type mockExtractor struct {
	result *core.ExtractResult
	err    error
	calls  int
	failN  int
}

type laneAwareExtractor struct {
	sources []core.AuthSource
	cookies []string
}

type extractorFunc func(url string, opts core.ExtractOptions) (*core.ExtractResult, error)

func (m *mockExtractor) Match(url string) bool {
	return true
}

func (m *mockExtractor) Extract(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	m.calls++
	if m.failN > 0 {
		if m.calls <= m.failN {
			return nil, m.err
		}
		return m.result, nil
	}
	return m.result, m.err
}

func (m *laneAwareExtractor) Match(url string) bool {
	return true
}

func (m *laneAwareExtractor) Extract(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	m.sources = append(m.sources, opts.Source)
	m.cookies = append(m.cookies, opts.Cookie)

	switch opts.Source {
	case core.AuthSourceNone:
		return nil, fmt.Errorf("HTTP 401: Unauthorized")
	case core.AuthSourceServer:
		if strings.TrimSpace(opts.Cookie) == "" {
			return nil, fmt.Errorf("HTTP 401: Unauthorized")
		}
		return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceServer}}, nil
	case core.AuthSourceClient:
		if strings.TrimSpace(opts.Cookie) == "" {
			return nil, fmt.Errorf("HTTP 401: Unauthorized")
		}
		return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
	default:
		return nil, fmt.Errorf("HTTP 401: Unauthorized")
	}
}

func (f extractorFunc) Match(url string) bool {
	return true
}

func (f extractorFunc) Extract(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	return f(url, opts)
}

func TestExtractionService_Extract_InvalidURL(t *testing.T) {
	reg := registry.NewRegistry()
	svc := NewService(reg, 30, 3, 0)

	invalidURLs := []string{
		"",
		"not-a-url",
		"ftp://example.com/video.mp4",
	}

	for _, url := range invalidURLs {
		_, err := svc.Extract(context.Background(), ExtractInput{URL: url})
		if err == nil {
			t.Errorf("expected error for URL %q, got nil", url)
			continue
		}
		if !errors.Is(err, apperrors.ErrInvalidURL) {
			t.Errorf("expected ErrInvalidURL for URL %q, got %v", url, err)
		}
	}
}

func TestExtractionService_Extract_UnknownNonNativeURLFallsBackToGenericExtractor(t *testing.T) {
	reg := registry.NewRegistry()

	fallbackCalls := 0
	svc := NewService(reg, 30, 3, 0, WithFallbackExtractorFactory(func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			fallbackCalls++
			if opts.Source != core.AuthSourceNone {
				t.Fatalf("expected fallback first lane to be unauthenticated, got %s", opts.Source)
			}
			return &core.ExtractResult{URL: url, Platform: "reddit"}, nil
		})
	}))

	result, err := svc.Extract(context.Background(), ExtractInput{
		URL: "https://reddit.com/r/golang/comments/123",
	})

	if err != nil {
		t.Fatalf("expected fallback success, got err=%v", err)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback extractor to be called once, got %d", fallbackCalls)
	}
	if result.Platform != "reddit" {
		t.Fatalf("expected metadata-driven platform from fallback extractor, got %q", result.Platform)
	}
}

func TestExtractionService_Extract_NativePatternMissDoesNotFallback(t *testing.T) {
	reg := registry.NewRegistry()

	fallbackCalls := 0
	svc := NewService(reg, 30, 3, 0, WithFallbackExtractorFactory(func() core.Extractor {
		fallbackCalls++
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			return &core.ExtractResult{URL: url, Platform: "generic"}, nil
		})
	}))

	_, err := svc.Extract(context.Background(), ExtractInput{
		URL: "https://instagram.com/p/abc123",
	})

	if err == nil {
		t.Fatalf("expected unsupported platform error for native URL miss")
	}
	if !errors.Is(err, apperrors.ErrUnsupportedPlatform) {
		t.Fatalf("expected ErrUnsupportedPlatform, got %v", err)
	}
	if fallbackCalls != 0 {
		t.Fatalf("expected fallback factory not to be called for native URL miss, got %d", fallbackCalls)
	}
}

func TestExtractionService_FallbackSkipsServerCookieLane(t *testing.T) {
	reg := registry.NewRegistry()

	var seenSources []core.AuthSource
	var seenCookies []string
	svc := NewService(reg, 30, 1, 0,
		WithServerCookies(map[string]string{"unknown-platform": "sid=server"}),
		WithFallbackExtractorFactory(func() core.Extractor {
			return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
				seenSources = append(seenSources, opts.Source)
				seenCookies = append(seenCookies, opts.Cookie)
				switch opts.Source {
				case core.AuthSourceNone:
					return nil, fmt.Errorf("HTTP 401: Unauthorized")
				case core.AuthSourceClient:
					return &core.ExtractResult{URL: url, Platform: "example"}, nil
				default:
					return nil, fmt.Errorf("unexpected auth source")
				}
			})
		}),
	)

	_, err := svc.Extract(context.Background(), ExtractInput{URL: "https://unknown.example/video/1", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected fallback lane progression success, got %v", err)
	}

	if len(seenSources) != 2 {
		t.Fatalf("expected exactly guest and user lanes, got %d calls", len(seenSources))
	}
	if seenSources[0] != core.AuthSourceNone || seenSources[1] != core.AuthSourceClient {
		t.Fatalf("expected lane order none->client, got %v", seenSources)
	}
	for _, cookie := range seenCookies {
		if cookie == "sid=server" {
			t.Fatalf("did not expect server cookie lane in fallback mode")
		}
	}
}

func TestExtractionService_Extract_Success(t *testing.T) {
	reg := registry.NewRegistry()

	expectedResult := &core.ExtractResult{
		URL:      "https://test.com/video/123",
		Platform: "testplatform",
	}

	mockExtFactory := func() core.Extractor {
		return &mockExtractor{result: expectedResult}
	}

	reg.Register("testplatform", []*regexp.Regexp{regexp.MustCompile(`^https://test\.com/.*$`)}, mockExtFactory)

	svc := NewService(reg, 30, 3, 0)
	result, err := svc.Extract(context.Background(), ExtractInput{
		URL:    "https://test.com/video/123",
		Cookie: "test_cookie=value",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.URL != expectedResult.URL {
		t.Errorf("expected URL %q, got %q", expectedResult.URL, result.URL)
	}

	if result.Platform != "testplatform" {
		t.Errorf("expected platform %q, got %q", "testplatform", result.Platform)
	}
}

func TestExtractionService_Extract_ContextCancellation(t *testing.T) {
	reg := registry.NewRegistry()
	svc := NewService(reg, 30, 3, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Extract(ctx, ExtractInput{
		URL: "https://test.com/video/123",
	})

	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "https:///path", true},
		{"with path", "https://example.com/path/to/video", false},
		{"with query", "https://example.com?v=123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateHTTPURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHTTPURL(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestTypedError(t *testing.T) {
	inner := errors.New("inner error")
	te := typedError{kind: apperrors.ErrInvalidURL, err: inner}

	if te.Error() != inner.Error() {
		t.Errorf("Error() = %v, want %v", te.Error(), inner.Error())
	}

	if !errors.Is(te, apperrors.ErrInvalidURL) {
		t.Error("expected errors.Is to match ErrInvalidURL")
	}
}

func TestExtractionService_Timeout(t *testing.T) {
	reg := registry.NewRegistry()
	svc := NewService(reg, 30, 3, 0)

	es, ok := svc.(*extractionService)
	if !ok {
		t.Fatal("expected extractionService type")
	}

	if es.timeoutSeconds != 30 {
		t.Errorf("expected timeout 30, got %d", es.timeoutSeconds)
	}
}

func TestCachedService(t *testing.T) {
	reg := registry.NewRegistry()

	mockExtFactory := func() core.Extractor {
		return &mockExtractor{
			result: &core.ExtractResult{
				URL:      "https://cached.com/video/123",
				Platform: "cachedplatform",
			},
		}
	}

	reg.Register("cachedplatform", []*regexp.Regexp{regexp.MustCompile(`^https://cached\.com/.*$`)}, mockExtFactory)

	baseSvc := NewService(reg, 30, 3, 0)
	cachedSvc := NewCachedService(baseSvc, cache.NewPlatformTTLConfig(100*time.Millisecond, nil))

	ctx := context.Background()
	input := ExtractInput{URL: "https://cached.com/video/123"}

	_, err := cachedSvc.Extract(ctx, input)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	_, err = cachedSvc.Extract(ctx, input)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	_, err = cachedSvc.Extract(ctx, input)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
}

func TestExtractionService_Retry_TransientThenSuccess(t *testing.T) {
	reg := registry.NewRegistry()

	mockExtFactory := func() core.Extractor {
		return &mockExtractor{
			result: &core.ExtractResult{URL: "https://retry.com/x"},
			err:    &net.DNSError{Err: "temp", Name: "example.com"},
			failN:  2,
		}
	}

	reg.Register("retry", []*regexp.Regexp{regexp.MustCompile(`^https://retry\.com/.*$`)}, mockExtFactory)

	svc := NewService(reg, 30, 3, 0)
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://retry.com/x"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if result == nil {
		t.Fatalf("expected result")
	}
	if result.Platform != "retry" {
		t.Fatalf("expected platform %q, got %q", "retry", result.Platform)
	}
}

func TestExtractionService_Retry_PermanentNoRetry(t *testing.T) {
	reg := registry.NewRegistry()

	me := &mockExtractor{err: fmt.Errorf("HTTP 401: Unauthorized")}
	mockExtFactory := func() core.Extractor { return me }

	reg.Register("auth", []*regexp.Regexp{regexp.MustCompile(`^https://auth\.com/.*$`)}, mockExtFactory)

	svc := NewService(reg, 30, 3, 0)
	_, err := svc.Extract(context.Background(), ExtractInput{URL: "https://auth.com/x"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if me.calls != 1 {
		t.Fatalf("expected 1 call, got %d", me.calls)
	}
}

func TestExtractionService_Retry_ExhaustedReturnsCategorized(t *testing.T) {
	reg := registry.NewRegistry()

	me := &mockExtractor{err: &net.DNSError{Err: "temp", Name: "example.com"}, failN: 10}
	mockExtFactory := func() core.Extractor { return me }

	reg.Register("retry", []*regexp.Regexp{regexp.MustCompile(`^https://retry2\.com/.*$`)}, mockExtFactory)

	svc := NewService(reg, 30, 3, 0)
	_, err := svc.Extract(context.Background(), ExtractInput{URL: "https://retry2.com/x"})
	if err == nil {
		t.Fatalf("expected error")
	}

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if appErr.Metadata == nil {
		t.Fatalf("expected metadata")
	}
	if appErr.Metadata["attempts"] != 3 {
		t.Fatalf("expected attempts=3, got %v", appErr.Metadata["attempts"])
	}
	if me.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", me.calls)
	}
}

func TestExtractionService_CookieLane_GuestThenServer(t *testing.T) {
	reg := registry.NewRegistry()

	me := &laneAwareExtractor{}
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane\.com/.*$`)}, func() core.Extractor {
		return me
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane.com/post/1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceServer {
		t.Fatalf("expected source server, got %s", result.Authentication.Source)
	}
	if len(me.sources) != 2 {
		t.Fatalf("expected 2 extraction calls, got %d", len(me.sources))
	}
	if me.sources[0] != core.AuthSourceNone || me.sources[1] != core.AuthSourceServer {
		t.Fatalf("expected lane order none->server, got %v", me.sources)
	}
}

func TestExtractionService_CookieLane_GuestServerThenUserProvided(t *testing.T) {
	reg := registry.NewRegistry()

	me := &mockExtractor{}
	me.err = fmt.Errorf("HTTP 401: Unauthorized")

	var calls int
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane2\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			switch calls {
			case 1:
				if opts.Source != core.AuthSourceNone {
					t.Fatalf("call1 expected none source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("HTTP 401: Unauthorized")
			case 2:
				if opts.Source != core.AuthSourceServer {
					t.Fatalf("call2 expected server source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("HTTP 401: Unauthorized")
			default:
				if opts.Source != core.AuthSourceClient {
					t.Fatalf("call3 expected client source, got %s", opts.Source)
				}
				if opts.Cookie != "sid=user" {
					t.Fatalf("call3 expected user cookie, got %q", opts.Cookie)
				}
				return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
			}
		})
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane2.com/post/1", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceClient {
		t.Fatalf("expected user-provided lane, got %s", result.Authentication.Source)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExtractionService_CookieLane_AdvancesOnNoMediaError(t *testing.T) {
	reg := registry.NewRegistry()

	calls := 0
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane-no-media\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			switch calls {
			case 1:
				if opts.Source != core.AuthSourceNone {
					t.Fatalf("call1 expected none source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("no media found in tweet")
			case 2:
				if opts.Source != core.AuthSourceServer {
					t.Fatalf("call2 expected server source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("no media found in tweet")
			default:
				if opts.Source != core.AuthSourceClient {
					t.Fatalf("call3 expected client source, got %s", opts.Source)
				}
				if opts.Cookie != "sid=user" {
					t.Fatalf("call3 expected user cookie, got %q", opts.Cookie)
				}
				return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
			}
		})
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane-no-media.com/post/1", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceClient {
		t.Fatalf("expected user-provided lane, got %s", result.Authentication.Source)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExtractionService_CookieLane_NoAdvanceOnNonAuthError(t *testing.T) {
	reg := registry.NewRegistry()

	calls := 0
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane3\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			return nil, &net.DNSError{Err: "temp", Name: "example.com"}
		})
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	_, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane3.com/post/1", Cookie: "sid=user"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected no lane advance for non-auth errors, got %d calls", calls)
	}
}

func TestExtractionService_CookieLane_AdvancesOnLoginRequiredError(t *testing.T) {
	reg := registry.NewRegistry()

	calls := 0
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane-login\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			switch calls {
			case 1:
				if opts.Source != core.AuthSourceNone {
					t.Fatalf("call1 expected none source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("login required to access this content")
			case 2:
				if opts.Source != core.AuthSourceServer {
					t.Fatalf("call2 expected server source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("login required to access this content")
			default:
				if opts.Source != core.AuthSourceClient {
					t.Fatalf("call3 expected client source, got %s", opts.Source)
				}
				if opts.Cookie != "sid=user" {
					t.Fatalf("call3 expected user cookie, got %q", opts.Cookie)
				}
				return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
			}
		})
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane-login.com/post/1", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceClient {
		t.Fatalf("expected user-provided lane, got %s", result.Authentication.Source)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExtractionService_CookieLane_AdvancesOnMediaNotFoundPrivateError(t *testing.T) {
	reg := registry.NewRegistry()

	calls := 0
	reg.Register("lane", []*regexp.Regexp{regexp.MustCompile(`^https://lane-private\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			switch calls {
			case 1:
				if opts.Source != core.AuthSourceNone {
					t.Fatalf("call1 expected none source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("media not found or private")
			case 2:
				if opts.Source != core.AuthSourceServer {
					t.Fatalf("call2 expected server source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("media not found or private")
			default:
				if opts.Source != core.AuthSourceClient {
					t.Fatalf("call3 expected client source, got %s", opts.Source)
				}
				if opts.Cookie != "sid=user" {
					t.Fatalf("call3 expected user cookie, got %q", opts.Cookie)
				}
				return &core.ExtractResult{URL: url, Platform: "lane", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
			}
		})
	})

	svc := NewService(reg, 30, 1, 0, WithServerCookies(map[string]string{"lane": "sid=server"}))
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://lane-private.com/post/1", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceClient {
		t.Fatalf("expected user-provided lane, got %s", result.Authentication.Source)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExtractionService_CookieLane_AdvancesOnYoutubeBotCheckError(t *testing.T) {
	reg := registry.NewRegistry()

	calls := 0
	reg.Register("youtube", []*regexp.Regexp{regexp.MustCompile(`^https://www\.youtube\.com/.*$`)}, func() core.Extractor {
		return extractorFunc(func(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
			calls++
			switch calls {
			case 1:
				if opts.Source != core.AuthSourceNone {
					t.Fatalf("call1 expected none source, got %s", opts.Source)
				}
				return nil, fmt.Errorf("yt-dlp execution failed: ERROR: [youtube] abc123: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication")
			default:
				if opts.Source != core.AuthSourceClient {
					t.Fatalf("call2 expected client source, got %s", opts.Source)
				}
				if opts.Cookie != "sid=user" {
					t.Fatalf("call2 expected user cookie, got %q", opts.Cookie)
				}
				return &core.ExtractResult{URL: url, Platform: "youtube", Authentication: core.Authentication{Used: true, Source: core.AuthSourceClient}}, nil
			}
		})
	})

	svc := NewService(reg, 30, 1, 0)
	result, err := svc.Extract(context.Background(), ExtractInput{URL: "https://www.youtube.com/watch?v=abc123", Cookie: "sid=user"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.Authentication.Source != core.AuthSourceClient {
		t.Fatalf("expected user-provided lane, got %s", result.Authentication.Source)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}
