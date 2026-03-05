package twitter

import (
	"encoding/json"
	"strings"
	"testing"

	"downaria-api/internal/extractors/core"
)

func TestExtractCt0Token(t *testing.T) {
	cookie := "auth_token=abc; ct0=token123; twid=u%3D1"
	if got := extractCt0Token(cookie); got != "token123" {
		t.Fatalf("expected ct0 token123, got %q", got)
	}
}

func TestBuildGraphQLURL(t *testing.T) {
	apiURL, err := buildGraphQLURL("2027715211566719316")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(apiURL, "/TweetDetail?") {
		t.Fatalf("expected TweetDetail endpoint, got %q", apiURL)
	}
	if !strings.Contains(apiURL, "focalTweetId") {
		t.Fatalf("expected focalTweetId variable, got %q", apiURL)
	}
}

func TestParseGraphQLPayload(t *testing.T) {
	raw := `{
  "data": {
    "threaded_conversation_with_injections_v2": {
      "instructions": [
        {
          "entries": [
            {
              "content": {
                "itemContent": {
                  "tweet_results": {
                    "result": {
                      "rest_id": "123",
                      "legacy": {
                        "full_text": "hello",
                        "favorite_count": 10,
                        "retweet_count": 2,
                        "reply_count": 1,
                        "extended_entities": {
                          "media": [
                            {
                              "type": "video",
                              "media_url_https": "https://pbs.twimg.com/media/example.jpg",
                              "video_info": {
                                "variants": [
                                  {
                                    "url": "https://video.twimg.com/example.mp4",
                                    "bitrate": 832000
                                  }
                                ]
                              }
                            }
                          ]
                        }
                      },
                      "core": {
                        "user_results": {
                          "result": {
                            "legacy": {
                              "name": "tester",
                              "screen_name": "tester_handle"
                            }
                          }
                        }
                      },
                      "views": {
                        "count": "99"
                      }
                    }
                  }
                }
              }
            }
          ]
        }
      ]
    }
  }
}`

	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	parsed, err := parseGraphQLPayload(payload, "123")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if parsed.TweetID != "123" {
		t.Fatalf("expected tweet id 123, got %q", parsed.TweetID)
	}
	if parsed.AuthorScreenName != "tester_handle" {
		t.Fatalf("expected author tester_handle, got %q", parsed.AuthorScreenName)
	}
	if len(parsed.MediaDetails) != 1 {
		t.Fatalf("expected 1 media item, got %d", len(parsed.MediaDetails))
	}
	if len(parsed.MediaDetails[0].VideoVariants) != 1 {
		t.Fatalf("expected 1 video variant, got %d", len(parsed.MediaDetails[0].VideoVariants))
	}
}

func TestBuildResult_RemovesHLSWhenProgressiveWithAudioExists(t *testing.T) {
	extractor := NewTwitterExtractor()
	result := extractor.buildResult("https://x.com/user/status/123", &twitterExtractData{
		TweetID:          "123",
		Text:             "test tweet",
		AuthorName:       "tester",
		AuthorScreenName: "tester_handle",
		MediaDetails: []twitterMediaDetail{
			{
				Type: "video",
				VideoVariants: []twitterVariant{
					{Bitrate: 800000, URL: "https://video.twimg.com/ext_tw_video/123/pu/vid/avc1/320x320/sample.mp4?tag=12"},
					{Bitrate: 800000, URL: "https://video.twimg.com/ext_tw_video/123/pu/pl/abc.m3u8?tag=12&v=cfc"},
				},
			},
		},
	}, core.ExtractOptions{})

	if len(result.Media) != 1 {
		t.Fatalf("expected 1 media entry, got %d", len(result.Media))
	}
	if len(result.Media[0].Variants) != 1 {
		t.Fatalf("expected 1 variant after hls filtering, got %d", len(result.Media[0].Variants))
	}
	if strings.Contains(result.Media[0].Variants[0].URL, ".m3u8") {
		t.Fatalf("did not expect hls variant when progressive with audio exists")
	}
}

func TestBuildResult_HLSVariantUsesM3U8Metadata(t *testing.T) {
	extractor := NewTwitterExtractor()
	result := extractor.buildResult("https://x.com/user/status/123", &twitterExtractData{
		TweetID:          "123",
		Text:             "test tweet",
		AuthorName:       "tester",
		AuthorScreenName: "tester_handle",
		MediaDetails: []twitterMediaDetail{
			{
				Type: "video",
				VideoVariants: []twitterVariant{
					{Bitrate: 0, URL: "https://video.twimg.com/ext_tw_video/123/pu/pl/abc.m3u8?tag=12&v=cfc"},
				},
			},
		},
	}, core.ExtractOptions{})

	if len(result.Media) != 1 {
		t.Fatalf("expected 1 media entry, got %d", len(result.Media))
	}
	if len(result.Media[0].Variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(result.Media[0].Variants))
	}

	var foundHLS bool
	for _, variant := range result.Media[0].Variants {
		if strings.Contains(variant.URL, ".m3u8") {
			foundHLS = true
			if variant.Format != "m3u8" {
				t.Fatalf("expected hls format m3u8, got %q", variant.Format)
			}
			if variant.Mime != "application/vnd.apple.mpegurl" {
				t.Fatalf("expected hls mime type, got %q", variant.Mime)
			}
			if !variant.RequiresProxy {
				t.Fatalf("expected hls variant to require proxy")
			}
		}
	}

	if !foundHLS {
		t.Fatalf("expected at least one hls variant")
	}
}

func TestBuildResult_UsesResolutionFromURLForQuality(t *testing.T) {
	extractor := NewTwitterExtractor()
	result := extractor.buildResult("https://x.com/user/status/123", &twitterExtractData{
		TweetID:          "123",
		Text:             "test tweet",
		AuthorName:       "tester",
		AuthorScreenName: "tester_handle",
		MediaDetails: []twitterMediaDetail{
			{
				Type: "video",
				VideoVariants: []twitterVariant{
					{Bitrate: 950000, URL: "https://video.twimg.com/ext_tw_video/123/pu/vid/avc1/480x852/sample.mp4?tag=12"},
					{Bitrate: 632000, URL: "https://video.twimg.com/ext_tw_video/123/pu/vid/avc1/320x568/sample.mp4?tag=12"},
				},
			},
		},
	}, core.ExtractOptions{})

	if len(result.Media) != 1 || len(result.Media[0].Variants) != 2 {
		t.Fatalf("expected one media with two variants")
	}

	if result.Media[0].Variants[0].Quality != "480p" {
		t.Fatalf("expected first quality 480p, got %q", result.Media[0].Variants[0].Quality)
	}
	if result.Media[0].Variants[0].Resolution != "480x852" {
		t.Fatalf("expected first resolution 480x852, got %q", result.Media[0].Variants[0].Resolution)
	}

	if result.Media[0].Variants[1].Quality != "320p" {
		t.Fatalf("expected second quality 320p, got %q", result.Media[0].Variants[1].Quality)
	}
	if result.Media[0].Variants[1].Resolution != "320x568" {
		t.Fatalf("expected second resolution 320x568, got %q", result.Media[0].Variants[1].Resolution)
	}
}

func TestIsHLSVariantURL(t *testing.T) {
	if !isHLSVariantURL("https://video.twimg.com/ext_tw_video/1/pu/pl/abc.m3u8?tag=12") {
		t.Fatalf("expected m3u8 url to be detected as hls")
	}
	if isHLSVariantURL("https://video.twimg.com/ext_tw_video/1/pu/vid/avc1/320x320/sample.mp4?tag=12") {
		t.Fatalf("did not expect mp4 url to be detected as hls")
	}
}

func TestQualityFromVariantURL(t *testing.T) {
	quality, resolution := qualityFromVariantURL("https://video.twimg.com/ext_tw_video/1/pu/vid/avc1/720x1280/sample.mp4?tag=12")
	if quality != "720p" {
		t.Fatalf("expected quality 720p, got %q", quality)
	}
	if resolution != "720x1280" {
		t.Fatalf("expected resolution 720x1280, got %q", resolution)
	}

	quality, resolution = qualityFromVariantURL("https://video.twimg.com/ext_tw_video/1/pu/pl/abc.m3u8?tag=12")
	if quality != "" || resolution != "" {
		t.Fatalf("expected empty quality/resolution for m3u8 playlist url")
	}
}
