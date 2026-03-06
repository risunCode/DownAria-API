package facebook

import (
	"strings"
	"testing"
)

func TestExtractMetadata_FallbackStatsFromTitleAndTitleCleanup(t *testing.T) {
	e := NewFacebookExtractor()
	html := `
<html>
<head>
  <meta property="og:title" content="83K views · 1.3K reactions | A very cool clip | Jane Doe">
</head>
<body>
  <script>"name":"Jane Doe","enable_reels_tab_deeplink":true</script>
</body>
</html>`

	m := e.extractMetadata(html, "https://facebook.com/reel/123")

	if m.Views != 83000 {
		t.Fatalf("expected views=83000, got %d", m.Views)
	}
	if m.Likes != 1300 {
		t.Fatalf("expected likes=1300, got %d", m.Likes)
	}
	if m.Title != "A very cool clip" {
		t.Fatalf("expected cleaned title without stats and trailing author, got %q", m.Title)
	}
}

func TestCleanFacebookTitle_RemovesTrailingAuthorAndNormalizesSeparators(t *testing.T) {
	raw := "My title|John Doe"
	cleaned := cleanFacebookTitle(raw, "John Doe")

	if cleaned != "My title" {
		t.Fatalf("expected title to remove trailing author, got %q", cleaned)
	}
}

func TestExtractMetadata_DecodesJSONUnicodeEscapes(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><head>
<meta property="og:title" content="V\u01b0\u01a1ng clip title">
<meta property="og:description" content="M\u00f4 t\u1ea3 ng\u1eafn">
</head><body>
<script>"name":"V\u01b0\u01a1ng","enable_reels_tab_deeplink":true</script>
</body></html>`

	m := e.extractMetadata(html, "https://facebook.com/reel/123")

	if m.Title != "Vương clip title" {
		t.Fatalf("expected decoded unicode title, got %q", m.Title)
	}
	if m.Description != "Mô tả ngắn" {
		t.Fatalf("expected decoded unicode description, got %q", m.Description)
	}
	if m.Author != "Vương" {
		t.Fatalf("expected decoded unicode author, got %q", m.Author)
	}
}

func TestExtractFormats_ProgressiveStoryURLs(t *testing.T) {
	e := NewFacebookExtractor()
	html := `"progressive_url":"https:\/\/video.cdn.test\/story_hd.mp4?_nc_cat=1","failure_reason":null,"metadata":{"quality":"HD"}`

	formats := e.extractFormats(html)
	if len(formats) == 0 {
		t.Fatalf("expected at least one format from progressive_url")
	}
	if formats[0].Quality != "HD" {
		t.Fatalf("expected quality HD, got %q", formats[0].Quality)
	}
	if formats[0].URL != "https://video.cdn.test/story_hd.mp4?_nc_cat=1" {
		t.Fatalf("unexpected URL after unescape: %q", formats[0].URL)
	}
}

func TestExtractFormats_DedupStoryVariantsByQualityAndPath(t *testing.T) {
	e := NewFacebookExtractor()
	html := strings.Join([]string{
		`"progressive_url":"https:\/\/video.cdn.test\/story_hd.mp4?token=a","failure_reason":null,"metadata":{"quality":"HD"}`,
		`"progressive_url":"https:\/\/video.cdn.test\/story_hd.mp4?token=b","failure_reason":null,"metadata":{"quality":"HD"}`,
		`"progressive_url":"https:\/\/video.cdn.test\/story_sd.mp4?token=1","failure_reason":null,"metadata":{"quality":"SD"}`,
		`"progressive_url":"https:\/\/video.cdn.test\/story_sd.mp4?token=2","failure_reason":null,"metadata":{"quality":"SD"}`,
	}, ",")

	formats := e.extractFormats(html)
	if len(formats) != 2 {
		t.Fatalf("expected 2 deduplicated variants (HD+SD), got %d", len(formats))
	}

	if formats[0].Quality != "HD" || formats[1].Quality != "SD" {
		t.Fatalf("expected HD then SD qualities, got %q and %q", formats[0].Quality, formats[1].Quality)
	}

	if formats[0].URL != "https://video.cdn.test/story_hd.mp4?token=a" {
		t.Fatalf("expected first HD URL to be kept, got %q", formats[0].URL)
	}
	if formats[1].URL != "https://video.cdn.test/story_sd.mp4?token=1" {
		t.Fatalf("expected first SD URL to be kept, got %q", formats[1].URL)
	}
}

func TestPreferStoryFormats_PicksHighestQuality(t *testing.T) {
	formats := []rawFormat{
		{Quality: "SD", URL: "https://video.cdn.test/story_sd.mp4"},
		{Quality: "HD", URL: "https://video.cdn.test/story_hd.mp4"},
	}

	got := preferStoryFormats(formats)
	if len(got) != 1 {
		t.Fatalf("expected single deduped story format, got %d", len(got))
	}
	if got[0].Quality != "HD" {
		t.Fatalf("expected highest quality HD, got %q", got[0].Quality)
	}
}

func TestPreferStoryFormats_FallbackLowestWhenNoHighQuality(t *testing.T) {
	formats := []rawFormat{
		{Quality: "540p", URL: "https://video.cdn.test/story_540.mp4"},
		{Quality: "360p", URL: "https://video.cdn.test/story_360.mp4"},
		{Quality: "480p", URL: "https://video.cdn.test/story_480.mp4"},
	}

	got := preferStoryFormats(formats)
	if len(got) != 1 {
		t.Fatalf("expected single fallback story format, got %d", len(got))
	}
	if got[0].Quality != "360p" {
		t.Fatalf("expected lowest quality fallback 360p, got %q", got[0].Quality)
	}
}

func TestCheckLoginRequired_DoesNotFailWhenProgressiveMediaPresent(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><body>login.php "progressive_url":"https:\/\/video.cdn.test\/story_sd.mp4"</body></html>`

	if err := e.checkLoginRequired(html); err != nil {
		t.Fatalf("expected no login-required error when progressive media exists, got %v", err)
	}
}

func TestExtractMetadata_StoryAuthorFromStoryBucketOwner(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><body><script>"story_bucket_owner":{"__typename":"User","id":"1001","name":"Story Author"}</script></body></html>`

	m := e.extractMetadata(html, "https://www.facebook.com/stories/ignored_user/123456789")
	if m.Author != "Story Author" {
		t.Fatalf("expected story author from story_bucket_owner, got %q", m.Author)
	}
}

func TestExtractMetadata_StoryTitleFallbackToStoryWhenMissing(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><body><script>"story_bucket_owner":{"name":"Jane"}</script></body></html>`

	m := e.extractMetadata(html, "https://www.facebook.com/stories/jane/123456789")
	if m.Title != "story" {
		t.Fatalf("expected story title fallback to 'story', got %q", m.Title)
	}
}

func TestExtractMetadata_StoryCapturesCreationTimestamp(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><body><script>"creation_time":1709518123</script></body></html>`

	m := e.extractMetadata(html, "https://www.facebook.com/stories/jane/123456789")
	if m.CreatedAt != "2024-03-04T02:08:43Z" {
		t.Fatalf("expected normalized createdAt from creation_time, got %q", m.CreatedAt)
	}
}

func TestExtractMetadata_StoryThumbnailFallbackFromInlinePayload(t *testing.T) {
	e := NewFacebookExtractor()
	html := `<html><body><script>"story_thumbnail":{"uri":"https:\/\/cdn.test\/thumb_story.jpg?_nc_cat=1"}</script></body></html>`

	m := e.extractMetadata(html, "https://www.facebook.com/stories/jane/123456789")
	if m.Thumbnail != "https://cdn.test/thumb_story.jpg?_nc_cat=1" {
		t.Fatalf("expected thumbnail fallback from story payload, got %q", m.Thumbnail)
	}
}

func TestBuildVariantFilename_StoryContainsAuthorMarkerWithoutTimestampID(t *testing.T) {
	e := NewFacebookExtractor()
	meta := fbMetadata{
		Author:    "Jane Doe",
		Title:     "",
		CreatedAt: "2026-03-03T18:31:40Z",
	}

	filename := e.buildVariantFilename(meta, "https://www.facebook.com/stories/jane.doe/99887766554433/", "mp4")

	if !strings.HasSuffix(filename, "[DownAria].mp4") {
		t.Fatalf("expected branded mp4 filename suffix, got %q", filename)
	}
	if !strings.Contains(filename, "jane_doe_story_[DownAria].mp4") {
		t.Fatalf("expected story filename to include author + story marker, got %q", filename)
	}
	if strings.Contains(filename, "20260303183140") {
		t.Fatalf("expected story filename to not include timestamp/id segment, got %q", filename)
	}
}
