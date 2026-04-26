package extract

import (
	"context"
	"net/url"
	"testing"
)

type testExtractor struct{ result *Result }

func (e testExtractor) Match(rawURL string) bool { return true }
func (e testExtractor) Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error) {
	return e.result, nil
}

type testUniversal struct{ result *Result }

func (u testUniversal) Extract(ctx context.Context, rawURL string, opts ExtractOptions) (*Result, error) {
	return u.result, nil
}

type testValidator struct{}

func (testValidator) Validate(ctx context.Context, rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func TestServicePrefersNativeExtractor(t *testing.T) {
	native := &Result{Title: "native", Media: []MediaItem{{Type: "video", Sources: []MediaSource{{URL: "https://cdn/native.mp4", HasVideo: true}}}}}
	universal := &Result{Title: "universal", Media: []MediaItem{{Type: "video", Sources: []MediaSource{{URL: "https://cdn/universal.mp4", HasVideo: true}}}}}
	svc := NewService(NewRegistry(Entry{Platform: "native", Extractor: testExtractor{result: native}}), testUniversal{result: universal}, testValidator{}, nil)
	got, err := svc.Extract(context.Background(), "https://example.com/post", ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "native" || got.ExtractProfile != "native" {
		t.Fatalf("got %#v", got)
	}
}

func TestServiceFallsBackToUniversal(t *testing.T) {
	universal := &Result{Title: "universal", Media: []MediaItem{{Type: "video", Sources: []MediaSource{{URL: "https://cdn/universal.mp4", HasVideo: true}}}}}
	svc := NewService(NewRegistry(), testUniversal{result: universal}, testValidator{}, nil)
	got, err := svc.Extract(context.Background(), "https://example.com/post", ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "universal" || got.ExtractProfile != "generic" {
		t.Fatalf("got %#v", got)
	}
}

func TestFinalizeResultFillsFilenameAndStreamProfile(t *testing.T) {
	result, err := FinalizeResult(&Result{Title: "Hello", Media: []MediaItem{{Type: "video", Sources: []MediaSource{{URL: "https://cdn/a.mp4", HasVideo: true}}}}}, "https://example.com/post", "x", "native")
	if err != nil {
		t.Fatal(err)
	}
	if result.Filename == "" || result.Media[0].Sources[0].StreamProfile == "" {
		t.Fatalf("not finalized: %#v", result)
	}
}
