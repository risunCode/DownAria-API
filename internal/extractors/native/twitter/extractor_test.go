package twitter

import (
	"encoding/json"
	"strings"
	"testing"
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
