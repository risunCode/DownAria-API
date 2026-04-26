package facebook

import "testing"

func TestMatchFacebookHosts(t *testing.T) {
	for _, rawURL := range []string{"https://www.facebook.com/watch?v=1", "https://fb.watch/abc"} {
		if !Match(rawURL) {
			t.Fatalf("expected match: %s", rawURL)
		}
	}
	if Match("https://example.com/watch") {
		t.Fatal("unexpected match")
	}
}

func TestFacebookMetadataHelpers(t *testing.T) {
	e := &Extractor{}
	html := `"name":"Alice","enable_reels_tab_deeplink":true "reaction_count":{"count":12 "comment_count":{"total_count":3 "share_count":{"count":2 "video_view_count":99 <meta property="og:title" content="99 views · Alice | Nice video">`
	m := e.extractMetadata(html, "https://facebook.com/reel/1")
	if m.Author != "Alice" || m.Views != 99 || m.Likes != 12 || m.Comments != 3 || m.Shares != 2 {
		t.Fatalf("metadata = %#v", m)
	}
}
