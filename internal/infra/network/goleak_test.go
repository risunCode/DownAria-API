package network

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestNoGoroutineLeak_StreamingAndPipeline(t *testing.T) {
	defer goleak.VerifyNone(t)

	sd := NewStreamingDownloader(NewBufferPool())
	_, _ = sd.StreamWithBuffer(context.Background(), io.NopCloser(bytes.NewReader([]byte("abc"))), io.Discard, "video/mp4")

	p := NewPipeline(NewBufferPool())
	_, _ = p.Run(context.Background(), bytes.NewReader([]byte("xyz")), io.Discard, "audio/mpeg")
}

func TestNoGoroutineLeak_FailurePaths(t *testing.T) {
	defer goleak.VerifyNone(t)

	sd := NewStreamingDownloader(NewBufferPool())
	_, _ = sd.StreamWithBuffer(
		context.Background(),
		io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("f"), VideoBufferSize*2))),
		&failAfterNWriter{failAfter: 1},
		"video/mp4",
	)

	pr, pw := io.Pipe()
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		chunk := bytes.Repeat([]byte("x"), 1024)
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
	_, _ = sd.StreamWithBuffer(ctx, pr, io.Discard, "video/mp4")

	select {
	case <-writeDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("pipe writer goroutine did not exit")
	}

	p := NewPipeline(NewBufferPool())
	_, _ = p.Run(
		context.Background(),
		bytes.NewReader(bytes.Repeat([]byte("p"), AudioBufferSize*2)),
		&failAfterNWriter{failAfter: 1},
		"audio/mpeg",
	)
}
