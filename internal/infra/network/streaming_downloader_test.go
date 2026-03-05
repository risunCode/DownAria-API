package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

var errDownstreamWrite = errors.New("downstream write failed")

type failAfterNWriter struct {
	failAfter int
	writes    int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errDownstreamWrite
	}
	w.writes++
	return len(p), nil
}

func TestStreamingDownloader_StreamWithBuffer(t *testing.T) {
	pool := NewBufferPool()
	d := NewStreamingDownloader(pool)
	src := io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), 64*1024)))
	var dst bytes.Buffer
	written, err := d.StreamWithBuffer(context.Background(), src, &dst, "video/mp4")
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if written != int64(dst.Len()) {
		t.Fatalf("written mismatch: %d != %d", written, dst.Len())
	}
}

func BenchmarkStreamingDownloader(b *testing.B) {
	pool := NewBufferPool()
	d := NewStreamingDownloader(pool)
	payload := bytes.Repeat([]byte("z"), 512*1024)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := io.NopCloser(bytes.NewReader(payload))
		var dst bytes.Buffer
		_, err := d.StreamWithBuffer(context.Background(), src, &dst, "video/mp4")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestStreamingDownloader_StreamWithBuffer_DownstreamError(t *testing.T) {
	pool := NewBufferPool()
	d := NewStreamingDownloader(pool)
	src := io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), VideoBufferSize*2)))
	dst := &failAfterNWriter{failAfter: 1}

	written, err := d.StreamWithBuffer(context.Background(), src, dst, "video/mp4")
	if !errors.Is(err, errDownstreamWrite) {
		t.Fatalf("expected downstream error, got: %v", err)
	}
	if written <= 0 {
		t.Fatalf("expected some bytes written before failure, got: %d", written)
	}
}

func TestStreamingDownloader_StreamWithBuffer_ContextCanceledMidStream(t *testing.T) {
	pool := NewBufferPool()
	d := NewStreamingDownloader(pool)

	pr, pw := io.Pipe()
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		chunk := bytes.Repeat([]byte("x"), 8*1024)
		for {
			if _, err := pw.Write(chunk); err != nil {
				_ = pw.Close()
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := d.StreamWithBuffer(ctx, pr, io.Discard, "video/mp4")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got: %v", err)
	}

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("writer goroutine did not stop after cancellation")
	}
}
