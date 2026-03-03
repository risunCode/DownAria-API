//go:build integration

package extractors

import (
	"context"
	"os"
	"testing"
	"time"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/extractors/registry"
)

func skipIfNotIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=1 to run.")
	}
}

func setupRegistry() *registry.Registry {
	reg := registry.NewRegistry()
	RegisterDefaultExtractors(reg)
	return reg
}

type testCase struct {
	name             string
	url              string
	expectedPlatform string
	timeout          time.Duration
	retries          int
}

func TestNativeExtractors_Integration(t *testing.T) {
	skipIfNotIntegration(t)

	testCases := []testCase{
		{
			name:             "Twitter Post",
			url:              "https://x.com/baghyeo51543827/status/2027976039448912258?s=20",
			expectedPlatform: "twitter",
			timeout:          30 * time.Second,
			retries:          1,
		},
		{
			name:             "Instagram Reel",
			url:              "https://www.instagram.com/reel/DU089BGk8OV/",
			expectedPlatform: "instagram",
			timeout:          30 * time.Second,
			retries:          1,
		},
		{
			name:             "Facebook Share",
			url:              "https://www.facebook.com/share/r/17xKEJFZeN/",
			expectedPlatform: "facebook",
			timeout:          90 * time.Second,
			retries:          3,
		},
		{
			name:             "TikTok Video",
			url:              "https://www.tiktok.com/@purrinchuu/video/7608581907361598738?is_from_webapp=1&sender_device=pc",
			expectedPlatform: "tiktok",
			timeout:          30 * time.Second,
			retries:          1,
		},
		{
			name:             "YouTube Music",
			url:              "https://music.youtube.com/watch?v=HOtiqLeNw5Q&list=PLHspEyts3wOjQRSQWg3JDRozPfCQjIDRp",
			expectedPlatform: "youtube",
			timeout:          45 * time.Second,
			retries:          1,
		},
		{
			name:             "Pixiv Artwork",
			url:              "https://www.pixiv.net/en/artworks/141367910",
			expectedPlatform: "pixiv",
			timeout:          30 * time.Second,
			retries:          1,
		},
	}

	reg := setupRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			extractor, platform, err := reg.GetExtractor(tc.url)
			if err != nil {
				t.Fatalf("failed to get extractor for URL %s: %v", tc.url, err)
			}
			if extractor == nil {
				t.Fatal("expected extractor to be found, got nil")
			}
			if platform != tc.expectedPlatform {
				t.Errorf("expected platform %q, got %q", tc.expectedPlatform, platform)
			}

			attempts := tc.retries
			if attempts < 1 {
				attempts = 1
			}

			var result *core.ExtractResult
			var lastErr error

			for attempt := 1; attempt <= attempts; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
				cookie := ""
				if tc.expectedPlatform == "facebook" {
					cookie = os.Getenv("FACEBOOK_COOKIE")
				}
				opts := core.ExtractOptions{
					Ctx:    ctx,
					Cookie: cookie,
					Source: core.AuthSourceNone,
				}
				if cookie != "" {
					opts.Source = core.AuthSourceClient
				}

				result, lastErr = extractor.Extract(tc.url, opts)
				cancel()

				if lastErr == nil {
					break
				}

				if attempt < attempts && tc.expectedPlatform == "facebook" {
					t.Logf("facebook attempt %d/%d failed: %v", attempt, attempts, lastErr)
					time.Sleep(2 * time.Second)
				}
			}

			if lastErr != nil {
				if tc.expectedPlatform == "facebook" {
					t.Skipf("facebook blocked/slow in current runner after %d attempts (timeout %s): %v", attempts, tc.timeout, lastErr)
					return
				}
				t.Fatalf("extraction failed for %s: %v", tc.url, lastErr)
			}

			if result == nil {
				t.Fatal("expected ExtractResult to not be nil")
			}
			if result.Platform != tc.expectedPlatform {
				t.Errorf("expected result.Platform to be %q, got %q", tc.expectedPlatform, result.Platform)
			}
			if result.URL != tc.url {
				t.Errorf("expected result.URL to be %q, got %q", tc.url, result.URL)
			}
			if len(result.Media) == 0 {
				t.Error("expected Media array to have at least one item")
			}
			for i, media := range result.Media {
				if len(media.Variants) == 0 {
					t.Errorf("expected Media[%d] to have at least one variant", i)
				}
			}
		})
	}
}

func TestRegistry_NoExtractorForInvalidURL(t *testing.T) {
	skipIfNotIntegration(t)

	reg := setupRegistry()
	invalidURL := "https://example.com/invalid/content"

	extractor, platform, err := reg.GetExtractor(invalidURL)
	if err == nil {
		t.Error("expected error for unsupported URL, got nil")
	}
	if extractor != nil {
		t.Error("expected nil extractor for unsupported URL")
	}
	if platform != "" {
		t.Errorf("expected empty platform for unsupported URL, got %q", platform)
	}
}
