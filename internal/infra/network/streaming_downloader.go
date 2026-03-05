package network

import (
	"context"
	"io"
	"sync"
)

type StreamingDownloader struct {
	buffers *BufferPool
}

func NewStreamingDownloader(pool *BufferPool) *StreamingDownloader {
	if pool == nil {
		pool = NewBufferPool()
	}
	return &StreamingDownloader{buffers: pool}
}

func (s *StreamingDownloader) StreamWithBuffer(ctx context.Context, upstream io.ReadCloser, downstream io.Writer, contentType string) (int64, error) {
	var closeOnce sync.Once
	closeUpstream := func() error {
		var err error
		closeOnce.Do(func() {
			err = upstream.Close()
		})
		return err
	}
	defer closeUpstream()

	pipeline := NewPipeline(s.buffers)
	return pipeline.run(ctx, upstream, downstream, contentType, closeUpstream)
}
