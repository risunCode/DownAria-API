package threads

import (
	"net/http"
	"strings"
	"testing"
)

func TestIsThreadsURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "https://www.threads.net/@user/post/ABC123", want: true},
		{url: "https://threads.com/@user/post/ABC123", want: true},
		{url: "https://www.instagram.com/reel/ABC123/", want: false},
	}

	for _, tt := range tests {
		if got := isThreadsURL(tt.url); got != tt.want {
			t.Fatalf("isThreadsURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeThreadsURL(t *testing.T) {
	normalized, err := normalizeThreadsURL("https://threads.com/@user/post/ABC123")
	if err != nil {
		t.Fatalf("normalizeThreadsURL returned error: %v", err)
	}
	if normalized != "https://www.threads.net/@user/post/ABC123" {
		t.Fatalf("unexpected normalized url: %s", normalized)
	}
}

func TestThreadsHTMLParsing(t *testing.T) {
	html := []byte(`
<html><head>
  <meta property="og:title" content="Sample Threads Post" />
  <meta content="Example description" property="og:description" />
  <meta property="og:image" content="https://scontent.cdninstagram.com/v/t51.2885-15/12345_n.jpg" />
</head><body>
  <button aria-label="Suka"><span>60</span></button>
  <button aria-label="Komentar"><span>12</span></button>
  <button aria-label="Bagikan"><span>3</span></button>
  <video src="https://scontent.cdninstagram.com/v/t50.2886-16/12345_n.mp4?stp=dst-mp4"></video>
  <script type="application/json" data-sjs>
    {"video":"https:\/\/scontent.cdninstagram.com\/v\/t50.2886-16\/99999_n.mp4?stp=dst-mp4","image":"https:\/\/scontent.cdninstagram.com\/v\/t51.2885-15\/77777_n.webp"}
  </script>
</body></html>`)

	meta := parseMetaProperties(html)
	if got := meta["og:title"]; got != "Sample Threads Post" {
		t.Fatalf("og:title mismatch: %q", got)
	}
	if got := meta["og:description"]; got != "Example description" {
		t.Fatalf("og:description mismatch: %q", got)
	}

	videos, images := collectMediaURLs(html, meta)
	if len(videos) == 0 {
		t.Fatal("expected at least one video URL")
	}
	if len(images) != 0 {
		t.Fatalf("expected images dropped for video post, got %d", len(images))
	}

	if likes := parseMetric(html, likesCountRegex); likes != 60 {
		t.Fatalf("likes mismatch: %d", likes)
	}
	if comments := parseMetric(html, commentsCountRegex); comments != 12 {
		t.Fatalf("comments mismatch: %d", comments)
	}
	if shares := parseMetric(html, sharesCountRegex); shares != 3 {
		t.Fatalf("shares mismatch: %d", shares)
	}
}

func TestParseEngagementFromJSONNumbers(t *testing.T) {
	html := []byte(`
<script type="application/json" data-sjs>
  {"like_count":24,"comment_count":5,"reshare_count":12,"play_count":777}
</script>`)

	likes, comments, shares, views := parseEngagement(html)
	if likes != 24 || comments != 5 || shares != 12 || views != 777 {
		t.Fatalf("engagement mismatch: likes=%d comments=%d shares=%d views=%d", likes, comments, shares, views)
	}
}

func TestCollectMediaURLs_DedupesVariantsAndDropsProfilePic(t *testing.T) {
	html := []byte(`
<html><body>
  <script type="application/json" data-sjs>
    {
      "video":"https:\/\/instagram.fcgk12-2.fna.fbcdn.net\/o1\/v\/t16\/f2\/m69\/AAA.mp4?efg=xpv_progressive_720",
      "img1":"https:\/\/instagram.fcgk12-1.fna.fbcdn.net\/v\/t51.82787-15\/BBBB_n.jpg?stp=dst-jpg_s150x150_tt6&efg=profile_pic",
      "img2":"https:\/\/instagram.fcgk12-1.fna.fbcdn.net\/v\/t51.82787-15\/CCCC_n.jpg?stp=c0.210.540.540a_dst-jpg_e15_s150x150_tt6&ig_cache_key=KEY1",
      "img3":"https:\/\/instagram.fcgk12-1.fna.fbcdn.net\/v\/t51.82787-15\/CCCC_n.jpg?stp=c0.210.540.540a_dst-jpg_e15_tt6&ig_cache_key=KEY1"
    }
  </script>
</body></html>`)

	videos, images := collectMediaURLs(html, map[string]string{})
	if len(videos) != 1 {
		t.Fatalf("expected exactly 1 video, got %d", len(videos))
	}
	if len(images) != 0 {
		t.Fatalf("expected 0 images when video exists, got %d", len(images))
	}
}

func TestCollectMediaURLs_ImagesOnlyDedupe(t *testing.T) {
	html := []byte(`
<html><body>
  <script type="application/json" data-sjs>
    {
      "img2":"https:\/\/instagram.fcgk12-1.fna.fbcdn.net\/v\/t51.82787-15\/CCCC_n.jpg?stp=c0.210.540.540a_dst-jpg_e15_s150x150_tt6&ig_cache_key=KEY1",
      "img3":"https:\/\/instagram.fcgk12-1.fna.fbcdn.net\/v\/t51.82787-15\/CCCC_n.jpg?stp=c0.210.540.540a_dst-jpg_e15_tt6&ig_cache_key=KEY1"
    }
  </script>
</body></html>`)

	videos, images := collectMediaURLs(html, map[string]string{})
	if len(videos) != 0 {
		t.Fatalf("expected 0 videos, got %d", len(videos))
	}
	if len(images) != 1 {
		t.Fatalf("expected deduped single image, got %d", len(images))
	}
	if images[0] == "" || !contains(images[0], "ig_cache_key=KEY1") {
		t.Fatalf("expected selected image to keep key variant, got %s", images[0])
	}
}

func contains(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || (len(s) >= len(sub) && strings.Contains(s, sub)))
}

func TestParseTotalFromContentRange(t *testing.T) {
	if got := parseTotalFromContentRange("bytes 0-0/12345"); got != 12345 {
		t.Fatalf("parseTotalFromContentRange mismatch: %d", got)
	}
}

func TestParseContentLengthFromResponse(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Content-Length": []string{"999"}}}
	if got := parseContentLengthFromResponse(resp); got != 999 {
		t.Fatalf("parseContentLengthFromResponse mismatch: %d", got)
	}
}
