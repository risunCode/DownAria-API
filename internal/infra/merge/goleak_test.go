package merge

import (
	"context"
	"errors"
	"io"
	"testing"

	"downaria-api/pkg/ffmpeg"
	"go.uber.org/goleak"
)

func TestNoGoroutineLeak_WorkerPool(t *testing.T) {
	defer goleak.VerifyNone(t)
	merger := NewStreamingMerger("missing-ffmpeg", 1024)
	merger.runner = &fakeMergeRunner{mergeFn: func(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error) {
		return nil, errors.New("boom")
	}}
	wp := NewMergeWorkerPool(1, 2, merger)
	job := &MergeJob{
		Ctx: t.Context(),
		Input: &MergeInput{
			VideoURL: "https://example.com/video",
			AudioURL: "https://example.com/audio",
		},
		Output:   io.Discard,
		ResultCh: make(chan error, 1),
	}
	_ = wp.Submit(job)
	<-job.ResultCh
	_ = wp.Shutdown(t.Context())
}
