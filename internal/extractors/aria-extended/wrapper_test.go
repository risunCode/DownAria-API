package ariaextended

import (
	"testing"

	"downaria-api/internal/extractors/core"
)

func TestResolvePlatform_UsesStaticPlatformForKnownExtractor(t *testing.T) {
	e := NewPythonExtractor("youtube")
	meta := &core.YTDLPDumpJSON{Extractor: "Reddit"}

	got := e.resolvePlatform(meta, "https://www.reddit.com/r/test")
	if got != "youtube" {
		t.Fatalf("expected static platform youtube, got %q", got)
	}
}

func TestResolvePlatform_UsesMetadataExtractorInFallbackMode(t *testing.T) {
	e := NewPythonExtractor("")
	meta := &core.YTDLPDumpJSON{Extractor: "Pinterest"}

	got := e.resolvePlatform(meta, "https://www.pinterest.com/pin/123")
	if got != "pinterest" {
		t.Fatalf("expected metadata extractor platform pinterest, got %q", got)
	}
}

func TestResolvePlatform_FallsBackToWebpageHost(t *testing.T) {
	e := NewPythonExtractor("")
	meta := &core.YTDLPDumpJSON{Extractor: "", WebpageURL: "https://m.reddit.com/r/golang/comments/abc"}

	got := e.resolvePlatform(meta, "https://unknown.tld/x")
	if got != "reddit" {
		t.Fatalf("expected platform from webpage host reddit, got %q", got)
	}
}

func TestBuildResultFromMeta_AggregatesMediaTypeFromIncludedSources(t *testing.T) {
	e := NewPythonExtractor("youtube")
	meta := &core.YTDLPDumpJSON{
		ID:       "abc123",
		Title:    "Audio post",
		Uploader: "tester",
		Formats: []core.YTDLPFormat{
			{FormatID: "video_dropped", URL: "", Ext: "mp4", VCodec: "avc1", ACodec: "none", Height: 720},
			{FormatID: "audio_kept", URL: "https://cdn.example/audio", MimeType: "audio/mp4; codecs=\"mp4a.40.2\"", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2"},
		},
	}

	result := e.buildResultFromMeta("https://example.com/watch?v=abc123", meta, core.ExtractOptions{})

	if result.MediaType != core.MediaTypeAudio {
		t.Fatalf("expected top-level mediaType audio, got %q", result.MediaType)
	}
	if len(result.Media) != 1 {
		t.Fatalf("expected exactly one media item, got %d", len(result.Media))
	}
	if result.Media[0].Type != core.MediaTypeAudio {
		t.Fatalf("expected media item type audio, got %q", result.Media[0].Type)
	}
	if len(result.Media[0].Variants) != 1 {
		t.Fatalf("expected one kept variant, got %d", len(result.Media[0].Variants))
	}
}

func TestBuildResultFromMeta_NormalizesVariantMimeAndExtension(t *testing.T) {
	e := NewPythonExtractor("youtube")
	meta := &core.YTDLPDumpJSON{
		ID:       "track1",
		Title:    "Track",
		Uploader: "artist",
		Formats: []core.YTDLPFormat{
			{FormatID: "a1", URL: "https://cdn.example/track", MimeType: "audio/mpeg; codecs=mp3", Ext: "mp4", VCodec: "none", ACodec: "mp3"},
		},
	}

	result := e.buildResultFromMeta("https://example.com/track1", meta, core.ExtractOptions{})
	if len(result.Media) != 1 || len(result.Media[0].Variants) != 1 {
		t.Fatalf("expected one media variant, got %+v", result.Media)
	}

	variant := result.Media[0].Variants[0]
	if variant.Mime != "audio/mpeg" {
		t.Fatalf("expected normalized mime audio/mpeg, got %q", variant.Mime)
	}
	if variant.Format != "mp3" {
		t.Fatalf("expected mime-derived format mp3, got %q", variant.Format)
	}
}

func TestBuildResultFromMeta_DedupePrefersKnownSizeForSameResolution(t *testing.T) {
	e := NewPythonExtractor("youtube")
	meta := &core.YTDLPDumpJSON{
		ID:       "vid1",
		Title:    "Video",
		Uploader: "creator",
		Formats: []core.YTDLPFormat{
			{
				FormatID: "high-bitrate-no-size",
				URL:      "https://cdn.example/v1",
				Ext:      "mp4",
				VCodec:   "avc1",
				ACodec:   "none",
				Height:   720,
				TBR:      2500,
			},
			{
				FormatID: "known-size",
				URL:      "https://cdn.example/v2",
				Ext:      "mp4",
				VCodec:   "avc1",
				ACodec:   "none",
				Height:   720,
				TBR:      1200,
				Filesize: 5_000_000,
			},
		},
	}

	result := e.buildResultFromMeta("https://example.com/watch?v=vid1", meta, core.ExtractOptions{})
	if len(result.Media) != 1 || len(result.Media[0].Variants) != 1 {
		t.Fatalf("expected one deduped variant, got %+v", result.Media)
	}

	variant := result.Media[0].Variants[0]
	if variant.FormatID != "known-size" {
		t.Fatalf("expected formatID known-size, got %q", variant.FormatID)
	}
	if variant.Size != 5_000_000 {
		t.Fatalf("expected variant size 5000000, got %d", variant.Size)
	}
}

func TestBuildResultFromMeta_KeepsNoHeightQualityVariantsSortedHighToLow(t *testing.T) {
	e := NewPythonExtractor("")
	meta := &core.YTDLPDumpJSON{
		ID:       "r34",
		Title:    "Rule34 sample",
		Uploader: "artist",
		Formats: []core.YTDLPFormat{
			{FormatID: "0", Quality: "360", URL: "https://cdn.example/360.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
			{FormatID: "3", Quality: "1080", URL: "https://cdn.example/1080.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
			{FormatID: "2", Quality: "720", URL: "https://cdn.example/720.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
			{FormatID: "1", Quality: "480", URL: "https://cdn.example/480.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none"},
		},
	}

	result := e.buildResultFromMeta("https://rule34video.com/video/3859321", meta, core.ExtractOptions{})
	if len(result.Media) != 1 {
		t.Fatalf("expected one media item, got %d", len(result.Media))
	}
	variants := result.Media[0].Variants
	if len(variants) != 4 {
		t.Fatalf("expected 4 variants, got %d", len(variants))
	}

	qualities := []string{variants[0].Quality, variants[1].Quality, variants[2].Quality, variants[3].Quality}
	expected := []string{"1080p", "720p", "480p", "360p"}
	for i := range expected {
		if qualities[i] != expected[i] {
			t.Fatalf("expected order %v got %v", expected, qualities)
		}
	}
}
