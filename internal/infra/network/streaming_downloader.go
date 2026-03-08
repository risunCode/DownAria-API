package network

import (
	"context"
	"io"
	"net/http"
)

// StreamingDownloader is an optimized streaming downloader that uses direct streaming
// with a single goroutine. This reduces overhead and improves throughput.
//
// Key features:
// - Single goroutine (no pipeline overhead)
// - No channel overhead
// - Progressive flushing for better perceived performance
// - Adaptive buffer sizing based on content type and length
// - Simple and maintainable code
type StreamingDownloader struct {
	buffers *BufferPool
}

// NewStreamingDownloader creates a new optimized streaming downloader
func NewStreamingDownloader(pool *BufferPool) *StreamingDownloader {
	if pool == nil {
		pool = NewBufferPool()
	}
	return &StreamingDownloader{buffers: pool}
}

// StreamWithBuffer streams data from upstream to downstream with optimized buffering
// and progressive flushing for better user experience.
//
// This implementation:
// - Uses a single goroutine (no pipeline overhead)
// - Supports progressive flushing via http.Flusher interface
// - Respects context cancellation
// - Uses adaptive buffer sizing based on content type and length
func (s *StreamingDownloader) StreamWithBuffer(
	ctx context.Context,
	upstream io.ReadCloser,
	downstream io.Writer,
	contentType string,
	contentLength int64,
) (int64, error) {
	// Determine optimal buffer size based on content type and length
	bufferSize := s.buffers.OptimalSizeForContentType(contentType, contentLength)
	buf := s.buffers.Get(bufferSize)
	defer s.buffers.Put(buf)

	// Check if downstream supports progressive flushing
	flusher, canFlush := downstream.(http.Flusher)

	var written int64

	for {
		// Check for context cancellation before each read
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		// Read from upstream
		n, readErr := upstream.Read(buf)

		if n > 0 {
			// Write to downstream
			nw, writeErr := downstream.Write(buf[:n])
			written += int64(nw)

			// Progressive flushing: send data to client immediately
			// This improves perceived performance and allows streaming to start faster
			if canFlush {
				flusher.Flush()
			}

			// Handle write errors
			if writeErr != nil {
				return written, writeErr
			}

			// Check for short writes
			if nw < n {
				return written, io.ErrShortWrite
			}
		}

		// Handle read completion
		if readErr == io.EOF {
			return written, nil
		}

		// Handle read errors
		if readErr != nil {
			return written, readErr
		}
	}
}

// StreamDirect is a convenience method that uses default buffer sizing
// This is kept for backward compatibility with existing code
func (s *StreamingDownloader) StreamDirect(
	ctx context.Context,
	upstream io.ReadCloser,
	downstream io.Writer,
	contentType string,
) (int64, error) {
	return s.StreamWithBuffer(ctx, upstream, downstream, contentType, 0)
}
