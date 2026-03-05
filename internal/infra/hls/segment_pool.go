package hls

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

type SegmentPoolMetrics struct {
	ActiveDownloads atomic.Int64
	TotalDownloads  atomic.Int64
	TotalBytes      atomic.Int64
	FailedDownloads atomic.Int64
}

type SegmentWorkerPool struct {
	workers   int
	semaphore chan struct{}
	client    *http.Client
	Metrics   SegmentPoolMetrics
}

func NewSegmentWorkerPool(client *http.Client, workers int) *SegmentWorkerPool {
	if workers <= 0 {
		workers = 5
	}
	if client == nil {
		client = &http.Client{}
	}
	return &SegmentWorkerPool{
		workers:   workers,
		semaphore: make(chan struct{}, workers),
		client:    client,
	}
}

func (p *SegmentWorkerPool) FetchSegment(ctx context.Context, target string, headers map[string]string) ([]byte, error) {
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	p.Metrics.ActiveDownloads.Add(1)
	defer p.Metrics.ActiveDownloads.Add(-1)
	p.Metrics.TotalDownloads.Add(1)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		p.Metrics.FailedDownloads.Add(1)
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.Metrics.FailedDownloads.Add(1)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		p.Metrics.FailedDownloads.Add(1)
		return nil, fmt.Errorf("segment returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		p.Metrics.FailedDownloads.Add(1)
		return nil, err
	}
	p.Metrics.TotalBytes.Add(int64(len(b)))
	return b, nil
}
