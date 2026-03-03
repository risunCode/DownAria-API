package registry

import (
	"regexp"
	"sync"
	"testing"

	"fetchmoona/internal/extractors/core"
)

type mockExtractor struct {
	name string
}

func (m *mockExtractor) Match(url string) bool {
	return true
}

func (m *mockExtractor) Extract(url string, opts core.ExtractOptions) (*core.ExtractResult, error) {
	return &core.ExtractResult{
		URL:      url,
		Platform: m.name,
	}, nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.extractors == nil {
		t.Fatal("extractors slice is nil")
	}
	if len(r.extractors) != 0 {
		t.Fatalf("expected 0 extractors, got %d", len(r.extractors))
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()

	pattern := regexp.MustCompile(`https?://example\.com/.*`)
	factory := func() core.Extractor {
		return &mockExtractor{name: "example"}
	}

	r.Register("example", []*regexp.Regexp{pattern}, factory)

	if len(r.extractors) != 1 {
		t.Fatalf("expected 1 extractor, got %d", len(r.extractors))
	}

	reg := r.extractors[0]
	if reg.platform != "example" {
		t.Errorf("expected platform 'example', got '%s'", reg.platform)
	}
	if len(reg.patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(reg.patterns))
	}
}

func TestGetExtractor_Found(t *testing.T) {
	r := NewRegistry()

	pattern := regexp.MustCompile(`https?://example\.com/.*`)
	factory := func() core.Extractor {
		return &mockExtractor{name: "example"}
	}

	r.Register("example", []*regexp.Regexp{pattern}, factory)

	extractor, platform, err := r.GetExtractor("https://example.com/video/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if extractor == nil {
		t.Fatal("extractor is nil")
	}
	if platform != "example" {
		t.Errorf("expected platform 'example', got '%s'", platform)
	}
}

func TestGetExtractor_NotFound(t *testing.T) {
	r := NewRegistry()

	pattern := regexp.MustCompile(`https?://example\.com/.*`)
	factory := func() core.Extractor {
		return &mockExtractor{name: "example"}
	}

	r.Register("example", []*regexp.Regexp{pattern}, factory)

	extractor, platform, err := r.GetExtractor("https://other.com/video/123")
	if err == nil {
		t.Fatal("expected error for non-matching URL, got nil")
	}
	if extractor != nil {
		t.Fatal("expected nil extractor for non-matching URL")
	}
	if platform != "" {
		t.Errorf("expected empty platform, got '%s'", platform)
	}
	expectedErr := "unsupported platform for URL"
	if err.Error()[:len(expectedErr)] != expectedErr {
		t.Errorf("expected error to contain '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestMultipleExtractors(t *testing.T) {
	r := NewRegistry()

	patterns1 := []*regexp.Regexp{regexp.MustCompile(`https?://youtube\.com/.*`)}
	patterns2 := []*regexp.Regexp{regexp.MustCompile(`https?://vimeo\.com/.*`)}
	patterns3 := []*regexp.Regexp{regexp.MustCompile(`https?://tiktok\.com/.*`)}

	r.Register("youtube", patterns1, func() core.Extractor {
		return &mockExtractor{name: "youtube"}
	})
	r.Register("vimeo", patterns2, func() core.Extractor {
		return &mockExtractor{name: "vimeo"}
	})
	r.Register("tiktok", patterns3, func() core.Extractor {
		return &mockExtractor{name: "tiktok"}
	})

	if len(r.extractors) != 3 {
		t.Fatalf("expected 3 extractors, got %d", len(r.extractors))
	}

	tests := []struct {
		url          string
		wantPlatform string
	}{
		{"https://youtube.com/watch?v=123", "youtube"},
		{"https://vimeo.com/123456", "vimeo"},
		{"https://tiktok.com/@user/video/123", "tiktok"},
	}

	for _, tt := range tests {
		_, platform, err := r.GetExtractor(tt.url)
		if err != nil {
			t.Errorf("unexpected error for %s: %v", tt.url, err)
			continue
		}
		if platform != tt.wantPlatform {
			t.Errorf("GetExtractor(%s) got platform %s, want %s", tt.url, platform, tt.wantPlatform)
		}
	}
}

func TestPatternPriority(t *testing.T) {
	r := NewRegistry()

	pattern1 := regexp.MustCompile(`https?://.*\.com/.*`)
	pattern2 := regexp.MustCompile(`https?://example\.com/.*`)

	r.Register("generic", []*regexp.Regexp{pattern1}, func() core.Extractor {
		return &mockExtractor{name: "generic"}
	})
	r.Register("specific", []*regexp.Regexp{pattern2}, func() core.Extractor {
		return &mockExtractor{name: "specific"}
	})

	_, platform, err := r.GetExtractor("https://example.com/video/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if platform != "generic" {
		t.Errorf("expected first matching pattern to win, got platform '%s', want 'generic'", platform)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	pattern := regexp.MustCompile(`https?://example\.com/.*`)
	factory := func() core.Extractor {
		return &mockExtractor{name: "example"}
	}

	r.Register("example", []*regexp.Regexp{pattern}, factory)

	var wg sync.WaitGroup
	numGoroutines := 100
	numIterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				url := "https://example.com/video/"
				if j%2 == 0 {
					url = "https://other.com/video/"
				}

				_, _, _ = r.GetExtractor(url)

				if j%10 == 0 {
					r.Register("dynamic", []*regexp.Regexp{regexp.MustCompile(`https?://dynamic\d+\.com/.*`)}, func() core.Extractor {
						return &mockExtractor{name: "dynamic"}
					})
				}
			}
		}(i)
	}

	wg.Wait()
}
