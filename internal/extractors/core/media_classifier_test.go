package core

import "testing"

func TestClassifyMedia_PrefersMimeOverExtension(t *testing.T) {
	classification := ClassifyMedia("audio/mpeg; codecs=mp3", "mp4", "none", "mp3")

	if classification.MediaType != MediaTypeAudio {
		t.Fatalf("expected media type audio, got %q", classification.MediaType)
	}
	if classification.Mime != "audio/mpeg" {
		t.Fatalf("expected normalized mime audio/mpeg, got %q", classification.Mime)
	}
	if classification.Extension != "mp3" {
		t.Fatalf("expected extension derived from mime mp3, got %q", classification.Extension)
	}
}

func TestClassifyMedia_FallsBackToExtension(t *testing.T) {
	classification := ClassifyMedia("", "WeBm", "none", "none")

	if classification.MediaType != MediaTypeVideo {
		t.Fatalf("expected media type video, got %q", classification.MediaType)
	}
	if classification.Mime != "video/webm" {
		t.Fatalf("expected mime video/webm, got %q", classification.Mime)
	}
	if classification.Extension != "webm" {
		t.Fatalf("expected normalized extension webm, got %q", classification.Extension)
	}
}

func TestClassifyMedia_FallsBackToCodec(t *testing.T) {
	classification := ClassifyMedia("", "", "none", "opus")

	if classification.MediaType != MediaTypeAudio {
		t.Fatalf("expected media type audio, got %q", classification.MediaType)
	}
	if classification.Mime != "audio/mp4" {
		t.Fatalf("expected fallback mime audio/mp4, got %q", classification.Mime)
	}
	if classification.Extension != "m4a" {
		t.Fatalf("expected extension from fallback mime m4a, got %q", classification.Extension)
	}
}

func TestAggregateMediaTypes_Priority(t *testing.T) {
	if got := AggregateMediaTypes([]MediaType{MediaTypeAudio, MediaTypeImage}); got != MediaTypeAudio {
		t.Fatalf("expected audio aggregate, got %q", got)
	}
	if got := AggregateMediaTypes([]MediaType{MediaTypeUnknown, MediaTypeVideo, MediaTypeAudio}); got != MediaTypeVideo {
		t.Fatalf("expected video aggregate, got %q", got)
	}
}
