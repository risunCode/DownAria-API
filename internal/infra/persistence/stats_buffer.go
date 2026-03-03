package persistence

import (
	"sync"
	"time"
)

type statsBuffer struct {
	recordCh chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}

	flushThreshold int
	flushInterval  time.Duration
	onFlush        func() error
	onError        func(error)

	stopOnce sync.Once
}

func newStatsBuffer(flushThreshold int, flushInterval time.Duration, onFlush func() error, onError func(error)) *statsBuffer {
	if flushThreshold < 1 {
		flushThreshold = 1
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	b := &statsBuffer{
		recordCh:       make(chan struct{}, flushThreshold*2),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		flushThreshold: flushThreshold,
		flushInterval:  flushInterval,
		onFlush:        onFlush,
		onError:        onError,
	}

	go b.run()
	return b
}

func (b *statsBuffer) Record() {
	select {
	case b.recordCh <- struct{}{}:
	default:
		return
	}
}

func (b *statsBuffer) Stop() {
	b.stopOnce.Do(func() {
		close(b.stopCh)
		<-b.doneCh
	})
}

func (b *statsBuffer) run() {
	ticker := time.NewTicker(b.flushInterval)
	defer func() {
		ticker.Stop()
		close(b.doneCh)
	}()

	pending := 0
	flushPending := func() {
		if pending == 0 {
			return
		}
		if err := b.onFlush(); err != nil && b.onError != nil {
			b.onError(err)
		}
		pending = 0
	}

	for {
		select {
		case <-b.recordCh:
			pending++
			if pending >= b.flushThreshold {
				flushPending()
			}
		case <-ticker.C:
			flushPending()
		case <-b.stopCh:
			flushPending()
			return
		}
	}
}
