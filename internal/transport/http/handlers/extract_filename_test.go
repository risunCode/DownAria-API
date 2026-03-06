package handlers

import (
	"strings"
	"testing"

	"downaria-api/internal/extractors/core"
)

func TestEnsureVariantFilenames_AlwaysRegeneratesWithMediaIndex(t *testing.T) {
	h := &Handler{}
	result := &core.ExtractResult{
		Author:  core.Author{Name: "Jane Doe"},
		Content: core.Content{Text: "My Clip"},
		Media: []core.Media{
			{
				Type: core.MediaTypeVideo,
				Variants: []core.Variant{
					{Format: "mp4"},
					{Filename: "already_set.mp4", Format: "mp4"},
				},
			},
			{
				Type: core.MediaTypeVideo,
				Variants: []core.Variant{
					{Format: "mp4"},
				},
			},
		},
	}

	h.ensureVariantFilenames(result)

	if got := result.Media[0].Variants[0].Filename; got == "" {
		t.Fatalf("expected missing filename to be generated")
	}
	if got := result.Media[0].Variants[0].Filename; !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("expected generated filename extension .mp4, got %q", got)
	}
	if got := result.Media[0].Variants[1].Filename; got == "already_set.mp4" {
		t.Fatalf("expected existing filename to be regenerated, got %q", got)
	}
	if got := result.Media[0].Variants[0].Filename; strings.Contains(got, "_1_") {
		t.Fatalf("expected media index 0 to hide numeric suffix, got %q", got)
	}
	if got := result.Media[1].Variants[0].Filename; !strings.Contains(got, "_2_[DownAria].mp4") {
		t.Fatalf("expected third variant to include _2_ suffix, got %q", got)
	}
}

func TestInferVariantExtension_FallbackOrder(t *testing.T) {
	if got := inferVariantExtension(&core.Variant{Format: "m4a", Mime: "audio/mpeg", URL: "https://a/b.mp3"}, core.MediaTypeAudio); got != "m4a" {
		t.Fatalf("expected format to win, got %q", got)
	}
	if got := inferVariantExtension(&core.Variant{Mime: "audio/mpeg", URL: "https://a/b.bin"}, core.MediaTypeAudio); got != "mp3" {
		t.Fatalf("expected mime fallback mp3, got %q", got)
	}
	if got := inferVariantExtension(&core.Variant{Mime: "application/vnd.apple.mpegurl; charset=utf-8", URL: "https://a/stream"}, core.MediaTypeVideo); got != "m3u8" {
		t.Fatalf("expected mime fallback m3u8, got %q", got)
	}
	if got := inferVariantExtension(&core.Variant{URL: "https://a/b.webm?x=1"}, core.MediaTypeVideo); got != "webm" {
		t.Fatalf("expected url fallback webm, got %q", got)
	}
	if got := inferVariantExtension(&core.Variant{}, core.MediaTypeImage); got != "jpg" {
		t.Fatalf("expected media type default jpg, got %q", got)
	}
}

func TestSmartTitleSeed_RemovesURLTagsAndNoiseWords(t *testing.T) {
	result := &core.ExtractResult{
		Content: core.Content{
			Text: "Astag follow me https://t.co/abc #viral @foo Cyrene cosplay source original",
		},
	}

	got := smartTitleSeed(result)
	if strings.Contains(strings.ToLower(got), "http") || strings.Contains(strings.ToLower(got), "follow") {
		t.Fatalf("expected noisy tokens removed, got %q", got)
	}
	if !strings.Contains(strings.ToLower(got), "cyrene") || !strings.Contains(strings.ToLower(got), "cosplay") {
		t.Fatalf("expected informative tokens preserved, got %q", got)
	}
}

func TestEnsureVariantFilenames_ReplacesUnknownWithContentBasedName(t *testing.T) {
	h := &Handler{}
	result := &core.ExtractResult{
		Platform: "facebook",
		Author:   core.Author{Name: "unknown"},
		Content:  core.Content{Text: "Cyrene Clip"},
		Media: []core.Media{
			{
				Type: core.MediaTypeVideo,
				Variants: []core.Variant{
					{Filename: "unknown_test_[DownAria].mp4", Format: "mp4"},
					{Filename: "facebook_cyrene_clip_[DownAria].mp4", Format: "mp4"},
					{Filename: "facebook_cyrene_clip_[DownAria].mp4", Format: "mp4"},
				},
			},
		},
	}

	h.ensureVariantFilenames(result)

	if got := result.Media[0].Variants[0].Filename; strings.HasPrefix(strings.ToLower(got), "unknown_") {
		t.Fatalf("expected unknown-prefixed filename to be replaced, got %q", got)
	}
	if got := result.Media[0].Variants[1].Filename; !strings.Contains(got, "cyrene_clip") {
		t.Fatalf("expected filename to include content-based title, got %q", got)
	}
	if got := result.Media[0].Variants[2].Filename; strings.Contains(got, "_1_") {
		t.Fatalf("expected media index 0 to hide numeric suffix, got %q", got)
	}
}

func TestEnsureVariantFilenames_FacebookStoryUsesStoriesDatePattern(t *testing.T) {
	h := &Handler{}
	result := &core.ExtractResult{
		URL:      "https://www.facebook.com/stories/jane.doe/99887766554433/",
		Platform: "facebook",
		Author:   core.Author{Name: "Jane Doe"},
		Content:  core.Content{CreatedAt: "2026-03-03T18:31:40Z"},
		Media: []core.Media{
			{
				Type: core.MediaTypeStory,
				Variants: []core.Variant{
					{Format: "mp4"},
					{Format: "mp4"},
				},
			},
		},
	}

	h.ensureVariantFilenames(result)

	if got := result.Media[0].Variants[0].Filename; got != "jane_doe_stories_20260303_[DownAria].mp4" {
		t.Fatalf("expected first story filename pattern, got %q", got)
	}
	if got := result.Media[0].Variants[1].Filename; got != "jane_doe_stories_20260303_1_[DownAria].mp4" {
		t.Fatalf("expected indexed story filename pattern, got %q", got)
	}
}
