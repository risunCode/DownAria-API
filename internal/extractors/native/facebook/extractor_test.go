package facebook

import "testing"

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
