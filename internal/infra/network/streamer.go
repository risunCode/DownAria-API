package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type StreamOptions struct {
	URL         string
	Headers     map[string]string
	RangeHeader string
}

type StreamResult struct {
	Body          io.ReadCloser
	ContentType   string
	ContentLength int64
	StatusCode    int
	Headers       map[string]string
}

type Streamer struct {
	client *http.Client
}

func NewStreamer() *Streamer {
	return &Streamer{
		client: GetDefaultClient(),
	}
}

func (s *Streamer) Stream(ctx context.Context, opts StreamOptions) (*StreamResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	if opts.RangeHeader != "" {
		req.Header.Set("Range", opts.RangeHeader)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &StreamResult{
		Body:          resp.Body,
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: resp.ContentLength,
		StatusCode:    resp.StatusCode,
		Headers:       headers,
	}, nil
}
