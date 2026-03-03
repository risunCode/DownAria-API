package core

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateFilename_RemovesURLAndSymbols(t *testing.T) {
	got := GenerateFilename(
		"907x109",
		"dancing little angels @aria #cos http://t.co/abc123",
		"2028782704276410654",
		"mp4",
	)

	if strings.Contains(got, "http") || strings.Contains(got, "t.co") {
		t.Fatalf("filename still contains url noise: %s", got)
	}
	if strings.Contains(got, "@") || strings.Contains(got, "#") {
		t.Fatalf("filename still contains social symbols: %s", got)
	}
	if !strings.HasSuffix(got, ".mp4") {
		t.Fatalf("expected mp4 suffix, got %s", got)
	}
}

func TestGenerateFilename_FallbackExtensionWhenInvalid(t *testing.T) {
	got := GenerateFilename("author", "title", "id", "")
	if !strings.HasSuffix(got, ".bin") {
		t.Fatalf("expected .bin fallback extension, got %s", got)
	}
}

func TestGenerateFilename_EmptyTitleDoesNotUseUntitled(t *testing.T) {
	got := GenerateFilename("author", "", "123", "mp4")
	if strings.Contains(got, "untitled") {
		t.Fatalf("should not contain untitled placeholder: %s", got)
	}
	if strings.Contains(got, "__") {
		t.Fatalf("should not contain empty component separators: %s", got)
	}
	if want := "author_123_[DownAria].mp4"; got != want {
		t.Fatalf("unexpected filename, want %s got %s", want, got)
	}
}

func TestBuildFilenameID_AuthorAndPost(t *testing.T) {
	got := BuildFilenameID("creator_01", "99887766")
	if want := "creator_01_99887766"; got != want {
		t.Fatalf("expected %s got %s", want, got)
	}
}

func TestBuildFilenameID_AuthorFallbackDatetime(t *testing.T) {
	prevNow := filenameNowFunc
	filenameNowFunc = func() time.Time {
		return time.Date(2026, 3, 3, 18, 31, 40, 0, time.UTC)
	}
	t.Cleanup(func() { filenameNowFunc = prevNow })

	got := BuildFilenameID("authorid", "")
	if want := "authorid_20260303183140"; got != want {
		t.Fatalf("expected %s got %s", want, got)
	}
}

func TestGenerateFilename_TitleWordCap(t *testing.T) {
	got := GenerateFilename(
		"author",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen",
		"postid",
		"mp4",
	)

	if strings.Contains(got, "sixteen") || strings.Contains(got, "seventeen") {
		t.Fatalf("title should be capped to 15 words, got: %s", got)
	}
	parts := strings.Split(got, "_")
	if len(parts) < 4 {
		t.Fatalf("unexpected filename format: %s", got)
	}

	// author + title words + postid + [DownAria].ext
	titleWords := len(parts) - 3
	if titleWords > 15 {
		t.Fatalf("title words should be capped to <= 15, got %d in %s", titleWords, got)
	}
}
