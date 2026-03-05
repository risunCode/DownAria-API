package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type slowFiniteReader struct {
	remaining int
	chunkSize int
	delay     time.Duration
}

func (r *slowFiniteReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	n := r.chunkSize
	if n > len(p) {
		n = len(p)
	}
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'p'
	}
	r.remaining -= n
	return n, nil
}

func TestPipeline_Run(t *testing.T) {
	p := NewPipeline(NewBufferPool())
	var dst bytes.Buffer
	written, err := p.Run(context.Background(), bytes.NewReader([]byte("abc")), &dst, "audio/mpeg")
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}
	if written != 3 || dst.String() != "abc" {
		t.Fatalf("unexpected output %d %q", written, dst.String())
	}
}

func TestPipeline_Run_DownstreamError(t *testing.T) {
	p := NewPipeline(NewBufferPool())
	src := bytes.NewReader(bytes.Repeat([]byte("b"), AudioBufferSize*2))
	dst := &failAfterNWriter{failAfter: 1}

	written, err := p.Run(context.Background(), src, dst, "audio/mpeg")
	if !errors.Is(err, errDownstreamWrite) {
		t.Fatalf("expected downstream error, got: %v", err)
	}
	if written <= 0 {
		t.Fatalf("expected partial writes before failure, got: %d", written)
	}
}

func TestPipeline_Run_ContextCanceledMidStream(t *testing.T) {
	p := NewPipeline(NewBufferPool())
	src := &slowFiniteReader{remaining: 512 * 1024, chunkSize: 8 * 1024, delay: 1 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(8 * time.Millisecond)
		cancel()
	}()

	_, err := p.Run(ctx, src, io.Discard, "video/mp4")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got: %v", err)
	}
}
