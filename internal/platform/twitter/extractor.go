package twitter

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"downaria-api/internal/extract"
)

var statusPattern = regexp.MustCompile(`/status/(\d+)`)

// Extractor extracts media from Twitter/X posts.
type Extractor struct{ client *Client }

// NewExtractor creates a new Twitter extractor with the given HTTP client.
func NewExtractor(client *Client) *Extractor { return &Extractor{client: client} }

// Match returns true if the URL is a valid Twitter/X URL.
func Match(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "x.com" && host != "www.x.com" && host != "twitter.com" && host != "www.twitter.com" {
		return false
	}
	return statusPattern.MatchString(parsed.Path)
}

func (e *Extractor) Match(rawURL string) bool { return Match(rawURL) }

func ExtractStatusID(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	matches := statusPattern.FindStringSubmatch(parsed.Path)
	if len(matches) != 2 {
		return "", fmt.Errorf("status id not found")
	}
	return matches[1], nil
}

// Extract extracts media metadata from a Twitter/X URL.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	statusID, err := ExtractStatusID(rawURL)
	if err != nil {
		return nil, extract.Wrap(extract.KindInvalidInput, extract.ErrMsgInvalidURL, err)
	}
	data, err := e.client.FetchTweet(ctx, statusID, opts)
	if err != nil {
		return nil, err
	}
	data = e.client.enrichSizes(ctx, data, rawURL)
	return extract.NewResultBuilder(rawURL, "twitter", "native").
		Title(strings.TrimSpace(data.Text)).
		Author(data.AuthorName, data.AuthorHandle).
		Engagement(data.Engagement.Views, data.Engagement.Likes, data.Engagement.Comments, data.Engagement.Shares, data.Engagement.Bookmarks).
		Media(data.Media).
		Build(), nil
}
