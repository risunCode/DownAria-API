package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type DownloadResult struct {
	Reader io.ReadCloser
	Size   int64
	Err    error
}

type ConcurrentDownloader struct {
	client *http.Client
}

func NewConcurrentDownloader(client *http.Client) *ConcurrentDownloader {
	if client == nil {
		client = GetDefaultClient()
	}
	return &ConcurrentDownloader{client: client}
}

func (d *ConcurrentDownloader) DownloadPair(ctx context.Context, videoURL, audioURL string, headers map[string]string) (*DownloadResult, *DownloadResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	videoCh := make(chan *DownloadResult, 1)
	audioCh := make(chan *DownloadResult, 1)

	go d.downloadToChannel(ctx, videoURL, headers, videoCh)
	go d.downloadToChannel(ctx, audioURL, headers, audioCh)

	var videoRes, audioRes *DownloadResult
	for i := 0; i < 2; i++ {
		select {
		case videoRes = <-videoCh:
			if videoRes.Err != nil {
				cancel()
				if audioRes != nil && audioRes.Reader != nil {
					_ = audioRes.Reader.Close()
				}
				return nil, nil, videoRes.Err
			}
		case audioRes = <-audioCh:
			if audioRes.Err != nil {
				cancel()
				if videoRes != nil && videoRes.Reader != nil {
					_ = videoRes.Reader.Close()
				}
				return nil, nil, audioRes.Err
			}
		case <-ctx.Done():
			if videoRes != nil && videoRes.Reader != nil {
				_ = videoRes.Reader.Close()
			}
			if audioRes != nil && audioRes.Reader != nil {
				_ = audioRes.Reader.Close()
			}
			return nil, nil, ctx.Err()
		}
	}

	return videoRes, audioRes, nil
}

func (d *ConcurrentDownloader) downloadToChannel(ctx context.Context, target string, headers map[string]string, ch chan<- *DownloadResult) {
	result := &DownloadResult{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.Err = err
		ch <- result
		return
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		result.Err = err
		ch <- result
		return
	}

	if resp.StatusCode >= http.StatusBadRequest {
		_ = resp.Body.Close()
		result.Err = fmt.Errorf("upstream returned status %d", resp.StatusCode)
		ch <- result
		return
	}

	result.Reader = resp.Body
	result.Size = resp.ContentLength
	ch <- result
}
