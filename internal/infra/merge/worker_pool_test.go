package merge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"downaria-api/pkg/ffmpeg"
)

type fakeMergeRunner struct {
	mergeFn func(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error)
}

func (f *fakeMergeRunner) StreamMerge(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error) {
	if f.mergeFn != nil {
		return f.mergeFn(ctx, opts)
	}
	return &ffmpeg.FFmpegResult{Stdout: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
}

func TestMergeWorkerPool_SubmitAndProcess(t *testing.T) {
	merger := NewStreamingMerger("", 1024)
	merger.runner = &fakeMergeRunner{}
	wp := NewMergeWorkerPool(1, 1, merger)
	defer func() { _ = wp.Shutdown(t.Context()) }()

	var out bytes.Buffer
	job := &MergeJob{
		Ctx: t.Context(),
		Input: &MergeInput{
			VideoURL: "https://example.com/video",
			AudioURL: "https://example.com/audio",
		},
		Output:   &out,
		ResultCh: make(chan error, 1),
	}
	if err := wp.Submit(job); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if err := <-job.ResultCh; err != nil {
		t.Fatalf("job failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected output")
	}
}

func TestMergeWorkerPool_QueueFull(t *testing.T) {
	wp := NewMergeWorkerPool(1, 1, NewStreamingMerger("", 1024))
	wp.merger.runner = &fakeMergeRunner{mergeFn: func(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error) {
		time.Sleep(50 * time.Millisecond)
		return &ffmpeg.FFmpegResult{Stdout: io.NopCloser(bytes.NewReader([]byte("x")))}, nil
	}}
	defer func() { _ = wp.Shutdown(t.Context()) }()

	job := func() *MergeJob {
		return &MergeJob{
			Ctx: t.Context(),
			Input: &MergeInput{
				VideoURL: "https://example.com/video",
				AudioURL: "https://example.com/audio",
			},
			Output:   io.Discard,
			ResultCh: make(chan error, 1),
		}
	}
	if err := wp.Submit(job()); err != nil {
		t.Fatalf("first submit should succeed: %v", err)
	}
	secondErr := wp.Submit(job())
	if secondErr == nil {
		if err := wp.Submit(job()); !errors.Is(err, ErrWorkerPoolQueueFull) {
			t.Fatalf("expected queue full error on third submit")
		}
	} else if !errors.Is(secondErr, ErrWorkerPoolQueueFull) {
		t.Fatalf("unexpected second submit error: %v", secondErr)
	}
	if wp.QueueCapacity() != 1 {
		t.Fatalf("expected queue capacity 1, got %d", wp.QueueCapacity())
	}
	if wp.EstimateRetryAfter() <= 0 {
		t.Fatalf("expected positive retry-after estimate")
	}
}

func TestMergeWorkerPool_OnStartReceivesQueueWait(t *testing.T) {
	merger := NewStreamingMerger("", 1024)
	merger.runner = &fakeMergeRunner{}
	wp := NewMergeWorkerPool(1, 1, merger)
	defer func() { _ = wp.Shutdown(t.Context()) }()

	waitCh := make(chan time.Duration, 1)
	job := &MergeJob{
		Ctx:    t.Context(),
		Input:  &MergeInput{VideoURL: "https://example.com/v", AudioURL: "https://example.com/a"},
		Output: io.Discard,
		OnStart: func(wait time.Duration) {
			waitCh <- wait
		},
		ResultCh: make(chan error, 1),
	}

	if err := wp.Submit(job); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if err := <-job.ResultCh; err != nil {
		t.Fatalf("job failed: %v", err)
	}
	select {
	case wait := <-waitCh:
		if wait < 0 {
			t.Fatalf("invalid wait duration: %v", wait)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected OnStart callback")
	}
}
