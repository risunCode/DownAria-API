package twitter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/netutil"
	"downaria-api/internal/platform/probe"
)

const DefaultSyndicationBaseURL = "https://cdn.syndication.twimg.com/tweet-result"

type Client struct {
	httpClient probe.Getter
	baseURL    string
}

type tweetData struct {
	ID, Text, AuthorName, AuthorHandle string
	Engagement                         extract.Engagement
	Media                              []extract.MediaItem
}

func NewClient(httpClient probe.Getter, baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultSyndicationBaseURL
	}
	return &Client{httpClient: httpClient, baseURL: baseURL}
}

func (c *Client) FetchTweet(ctx context.Context, statusID string, opts extract.ExtractOptions) (*tweetData, error) {
	if c == nil || c.httpClient == nil {
		return nil, extract.Wrap(extract.KindInternal, "twitter client is not configured", nil)
	}
	apiURL := fmt.Sprintf("%s?id=%s&token=0", strings.TrimRight(c.baseURL, "/"), url.QueryEscape(statusID))
	headers := map[string]string{"Accept": "application/json", "Referer": "https://platform.twitter.com/"}
	if opts.UseAuth && opts.CookieHeader != "" && netutil.SameHost("https://twitter.com/i/status/"+statusID, apiURL) {
		headers["Cookie"] = opts.CookieHeader
	}
	resp, err := c.httpClient.Get(ctx, apiURL, headers)
	if err != nil {
		return nil, extract.Wrap(extract.KindUpstreamFailure, "twitter upstream request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, extract.Wrap(extract.KindUpstreamFailure, strings.TrimSpace(string(body)), fmt.Errorf("unexpected twitter status: %d", resp.StatusCode))
	}
	return decodeSyndication(resp.Body, statusID)
}

func (c *Client) enrichSizes(ctx context.Context, data *tweetData, referer string) *tweetData {
	if c == nil || c.httpClient == nil || data == nil {
		return data
	}
	for i := range data.Media {
		for j := range data.Media[i].Sources {
			source := &data.Media[i].Sources[j]
			size := probe.SizeWithGetter(ctx, c.httpClient, source.URL, map[string]string{"Referer": referer})
			source.FileSizeBytes = size
			data.Media[i].FileSizeBytes += size
		}
	}
	return data
}
