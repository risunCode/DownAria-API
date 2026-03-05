package merge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var ErrWorkerPoolClosed = errors.New("worker pool closed")
var ErrWorkerPoolQueueFull = errors.New("worker pool queue is full")

type MergeJob struct {
	Ctx        context.Context
	Input      *MergeInput
	Output     io.Writer
	ResultCh   chan error
	OnStart    func(wait time.Duration)
	enqueuedAt time.Time
}

type WorkerPoolMetrics struct {
	ActiveWorkers atomic.Int64
	QueueDepth    atomic.Int64
	Processed     atomic.Int64
	Failed        atomic.Int64
}

type MergeWorkerPool struct {
	workers           int
	queue             chan *MergeJob
	merger            *StreamingMerger
	wg                sync.WaitGroup
	closed            atomic.Bool
	totalProcessNanos atomic.Int64
	Metrics           WorkerPoolMetrics
}

func NewMergeWorkerPool(workers, queueSize int, merger *StreamingMerger) *MergeWorkerPool {
	if workers <= 0 {
		workers = 3
	}
	if queueSize < 0 {
		queueSize = 10
	}
	if merger == nil {
		merger = NewStreamingMerger("", 512*1024*1024)
	}
	wp := &MergeWorkerPool{
		workers: workers,
		queue:   make(chan *MergeJob, queueSize),
		merger:  merger,
	}
	for i := 0; i < workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
	return wp
}

func (wp *MergeWorkerPool) Submit(job *MergeJob) error {
	if wp.closed.Load() {
		return ErrWorkerPoolClosed
	}
	if job == nil || job.Input == nil || job.Output == nil {
		return fmt.Errorf("invalid merge job")
	}
	if job.Ctx == nil {
		job.Ctx = context.Background()
	}
	if job.enqueuedAt.IsZero() {
		job.enqueuedAt = time.Now()
	}
	select {
	case wp.queue <- job:
		wp.Metrics.QueueDepth.Store(int64(len(wp.queue)))
		return nil
	default:
		return ErrWorkerPoolQueueFull
	}
}

func (wp *MergeWorkerPool) worker() {
	defer wp.wg.Done()
	for job := range wp.queue {
		start := time.Now()
		queueWait := start.Sub(job.enqueuedAt)
		if job.OnStart != nil {
			job.OnStart(queueWait)
		}
		wp.Metrics.ActiveWorkers.Add(1)
		wp.Metrics.QueueDepth.Store(int64(len(wp.queue)))
		_, err := wp.merger.MergeAndStream(job.Ctx, job.Input, job.Output)
		wp.totalProcessNanos.Add(time.Since(start).Nanoseconds())
		if err != nil {
			wp.Metrics.Failed.Add(1)
		}
		wp.Metrics.Processed.Add(1)
		wp.Metrics.ActiveWorkers.Add(-1)
		wp.Metrics.QueueDepth.Store(int64(len(wp.queue)))
		if job.ResultCh != nil {
			job.ResultCh <- err
		}
	}
}

func (wp *MergeWorkerPool) QueueDepth() int {
	if wp == nil {
		return 0
	}
	return len(wp.queue)
}

func (wp *MergeWorkerPool) QueueCapacity() int {
	if wp == nil {
		return 0
	}
	return cap(wp.queue)
}

func (wp *MergeWorkerPool) EstimateRetryAfter() time.Duration {
	if wp == nil {
		return 0
	}
	return wp.estimateRetryAfterDepth(wp.QueueDepth() + 1)
}

func (wp *MergeWorkerPool) estimateRetryAfterDepth(depth int) time.Duration {
	if depth <= 0 {
		return 0
	}
	if wp.workers <= 0 {
		return 2 * time.Second
	}
	processed := wp.Metrics.Processed.Load()
	avg := 2 * time.Second
	if processed > 0 {
		nanos := wp.totalProcessNanos.Load()
		if nanos > 0 {
			avg = time.Duration(nanos / processed)
		}
	}
	if avg < 500*time.Millisecond {
		avg = 500 * time.Millisecond
	}
	waves := int(math.Ceil(float64(depth) / float64(wp.workers)))
	if waves < 1 {
		waves = 1
	}
	return time.Duration(waves) * avg
}

func (wp *MergeWorkerPool) Shutdown(ctx context.Context) error {
	if !wp.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(wp.queue)
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("worker pool shutdown timeout")
	}
}
