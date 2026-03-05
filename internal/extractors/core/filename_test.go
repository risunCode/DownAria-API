package core

import (
	"strings"
	"testing"
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
	if want := "author_media_123_[DownAria].mp4"; got != want {
		t.Fatalf("unexpected filename, want %s got %s", want, got)
	}
}

func TestBuildFilenameID_DisabledReturnsEmpty(t *testing.T) {
	got := BuildFilenameID("creator_01", "99887766")
	if want := ""; got != want {
		t.Fatalf("expected %s got %s", want, got)
	}
}

func TestGenerateFilename_NoTitleCap(t *testing.T) {
	got := GenerateFilename(
		"author",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen",
		"postid",
		"mp4",
	)

	if !strings.Contains(got, "sixteen") || !strings.Contains(got, "seventeen") {
		t.Fatalf("title should not be capped, got: %s", got)
	}
	if !strings.Contains(got, "_postid_[DownAria].mp4") {
		t.Fatalf("expected index token and brand suffix, got: %s", got)
	}
}

func TestGenerateFilename_IndexZeroHidden(t *testing.T) {
	got := GenerateFilename("author", "title", "0", "mp4")
	if strings.Contains(got, "_0_") {
		t.Fatalf("index 0 should be hidden, got %s", got)
	}
	if want := "author_title_[DownAria].mp4"; got != want {
		t.Fatalf("unexpected filename for index 0, want %s got %s", want, got)
	}
}
