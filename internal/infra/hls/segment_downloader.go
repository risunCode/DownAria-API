package hls

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafov/m3u8"
)

type HLSDownloadProgress struct {
	StartTime         time.Time
	TotalSegments     int
	CompletedSegments atomic.Int64
	FailedSegments    atomic.Int64
	TotalBytes        atomic.Int64
	DownloadedBytes   atomic.Int64
}

func (p *HLSDownloadProgress) Progress() float64 {
	if p.TotalSegments <= 0 {
		return 0
	}
	return float64(p.CompletedSegments.Load()) / float64(p.TotalSegments)
}

func (p *HLSDownloadProgress) SpeedBytesPerSec() float64 {
	elapsed := time.Since(p.StartTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(p.DownloadedBytes.Load()) / elapsed
}

type SegmentDownloadResult struct {
	Index    int
	Data     []byte
	Size     int64
	Duration time.Duration
	Attempts int
	Error    error
}

type SegmentDownloader struct {
	client     *http.Client
	pool       *SegmentWorkerPool
	parser     *Parser
	maxRetries int
}

const maxPlaylistResolveDepth = 8

func NewSegmentDownloader(client *http.Client, workers, retries int) *SegmentDownloader {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if retries <= 0 {
		retries = 3
	}
	return &SegmentDownloader{
		client:     client,
		pool:       NewSegmentWorkerPool(client, workers),
		parser:     NewParser(),
		maxRetries: retries,
	}
}

func (d *SegmentDownloader) DownloadAndConcatenate(ctx context.Context, playlistURL string, headers map[string]string) (io.ReadCloser, int64, *HLSDownloadProgress, error) {
	segmentURLs, err := d.resolveSegmentURLs(ctx, playlistURL, headers)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("resolve segments from playlist %q: %w", playlistURL, err)
	}
	if len(segmentURLs) == 0 {
		return nil, 0, nil, fmt.Errorf("resolve segments from playlist %q: no segments found", playlistURL)
	}

	progress := &HLSDownloadProgress{StartTime: time.Now(), TotalSegments: len(segmentURLs)}
	results := make([]*SegmentDownloadResult, len(segmentURLs))
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for i := range segmentURLs {
		wg.Add(1)
		idx := i
		seg := segmentURLs[i]
		go func() {
			defer wg.Done()
			res := d.downloadSegmentWithRetry(ctx, seg, headers, idx)
			if res.Error != nil {
				progress.FailedSegments.Add(1)
				select {
				case errCh <- res.Error:
				default:
				}
				cancel()
				return
			}
			results[idx] = res
			progress.CompletedSegments.Add(1)
			progress.DownloadedBytes.Add(res.Size)
		}()
	}

	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return nil, 0, progress, err
	}

	reader, total, err := concatenateSegments(results)
	if err != nil {
		return nil, 0, progress, err
	}
	progress.TotalBytes.Store(total)
	return reader, total, progress, nil
}

func (d *SegmentDownloader) resolveSegmentURLs(ctx context.Context, playlistURL string, headers map[string]string) ([]string, error) {
	return d.resolveSegmentURLsRecursive(ctx, playlistURL, headers, 0, map[string]struct{}{})
}

func (d *SegmentDownloader) resolveSegmentURLsRecursive(ctx context.Context, playlistURL string, headers map[string]string, depth int, visited map[string]struct{}) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("playlist %q resolve canceled: %w", playlistURL, err)
	}
	if depth > maxPlaylistResolveDepth {
		return nil, fmt.Errorf("playlist %q resolve depth exceeded (max=%d)", playlistURL, maxPlaylistResolveDepth)
	}
	if _, ok := visited[playlistURL]; ok {
		return nil, fmt.Errorf("playlist %q resolve loop detected", playlistURL)
	}

	visited[playlistURL] = struct{}{}
	playlist, listType, err := d.fetchAndParsePlaylist(ctx, playlistURL, headers)
	if err != nil {
		return nil, err
	}

	if listType == m3u8.MEDIA {
		segmentURLs, err := extractSegmentURLs(playlist, listType, playlistURL)
		if err != nil {
			return nil, fmt.Errorf("playlist %q extract media segments: %w", playlistURL, err)
		}
		if len(segmentURLs) == 0 {
			return nil, fmt.Errorf("playlist %q has no media segments", playlistURL)
		}
		return segmentURLs, nil
	}

	if listType != m3u8.MASTER {
		return nil, fmt.Errorf("playlist %q unsupported type %d", playlistURL, listType)
	}

	master := playlist.(*m3u8.MasterPlaylist)
	variants := sortedVariants(master)
	if len(variants) == 0 {
		return nil, fmt.Errorf("playlist %q master has no variants", playlistURL)
	}

	variantErrs := make([]error, 0, len(variants))
	for _, variant := range variants {
		variantURL := ResolveURL(variant.URI, playlistURL)
		branchVisited := cloneVisited(visited)
		segmentURLs, variantErr := d.resolveSegmentURLsRecursive(ctx, variantURL, headers, depth+1, branchVisited)
		if variantErr == nil {
			return segmentURLs, nil
		}
		variantErrs = append(variantErrs, fmt.Errorf("variant %q: %w", variantURL, variantErr))
		if errors.Is(variantErr, context.Canceled) || errors.Is(variantErr, context.DeadlineExceeded) {
			break
		}
	}

	return nil, fmt.Errorf("playlist %q master resolution failed after %d variants: %w", playlistURL, len(variants), errors.Join(variantErrs...))
}

func (d *SegmentDownloader) fetchAndParsePlaylist(ctx context.Context, playlistURL string, headers map[string]string) (m3u8.Playlist, m3u8.ListType, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, m3u8.ListType(0), fmt.Errorf("playlist %q build request: %w", playlistURL, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, m3u8.ListType(0), fmt.Errorf("playlist %q fetch: %w", playlistURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, m3u8.ListType(0), fmt.Errorf("playlist %q fetch: status %d", playlistURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, m3u8.ListType(0), fmt.Errorf("playlist %q read body: %w", playlistURL, err)
	}

	playlist, listType, err := d.parser.ParsePlaylist(body)
	if err != nil {
		return nil, listType, fmt.Errorf("playlist %q parse: %w", playlistURL, err)
	}

	return playlist, listType, nil
}

func (d *SegmentDownloader) downloadSegmentWithRetry(ctx context.Context, target string, headers map[string]string, index int) *SegmentDownloadResult {
	res := &SegmentDownloadResult{Index: index}
	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				res.Error = ctx.Err()
				res.Attempts = attempt
				return res
			case <-time.After(backoff):
			}
		}
		start := time.Now()
		b, err := d.pool.FetchSegment(ctx, target, headers)
		if err == nil {
			res.Data = b
			res.Size = int64(len(b))
			res.Duration = time.Since(start)
			res.Attempts = attempt + 1
			return res
		}
		lastErr = err
	}
	res.Error = fmt.Errorf("failed after %d retries: %w", d.maxRetries, lastErr)
	res.Attempts = d.maxRetries + 1
	return res
}

func extractSegmentURLs(playlist m3u8.Playlist, listType m3u8.ListType, baseURL string) ([]string, error) {
	switch listType {
	case m3u8.MEDIA:
		media := playlist.(*m3u8.MediaPlaylist)
		out := make([]string, 0)
		for _, seg := range media.Segments {
			if seg == nil || seg.URI == "" {
				continue
			}
			out = append(out, ResolveURL(seg.URI, baseURL))
		}
		return out, nil
	case m3u8.MASTER:
		return nil, errors.New("master playlist requires recursive resolution")
	default:
		return nil, errors.New("unsupported playlist type")
	}
}

func sortedVariants(master *m3u8.MasterPlaylist) []*m3u8.Variant {
	variants := make([]*m3u8.Variant, 0, len(master.Variants))
	for _, variant := range master.Variants {
		if variant == nil || strings.TrimSpace(variant.URI) == "" {
			continue
		}
		variants = append(variants, variant)
	}

	sort.SliceStable(variants, func(i, j int) bool {
		if variants[i].Bandwidth == variants[j].Bandwidth {
			return variants[i].URI < variants[j].URI
		}
		return variants[i].Bandwidth > variants[j].Bandwidth
	})

	return variants
}

func cloneVisited(visited map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(visited))
	for key := range visited {
		out[key] = struct{}{}
	}
	return out
}

func concatenateSegments(results []*SegmentDownloadResult) (io.ReadCloser, int64, error) {
	var total int64
	for _, r := range results {
		if r == nil || r.Error != nil || r.Data == nil {
			return nil, 0, errors.New("missing segment data")
		}
		total += int64(len(r.Data))
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := range results {
			if _, err := pw.Write(results[i].Data); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			results[i].Data = nil
		}
	}()
	return pr, total, nil
}
