package network

import (
	"context"
	"io"
	"sync"
)

type PipelineChunk struct {
	Data []byte
	Err  error
}

type Pipeline struct {
	BufferPool *BufferPool
}

func NewPipeline(pool *BufferPool) *Pipeline {
	if pool == nil {
		pool = NewBufferPool()
	}
	return &Pipeline{BufferPool: pool}
}

func (p *Pipeline) Run(ctx context.Context, src io.Reader, dst io.Writer, contentType string) (int64, error) {
	return p.run(ctx, src, dst, contentType, nil)
}

func (p *Pipeline) run(ctx context.Context, src io.Reader, dst io.Writer, contentType string, abort func() error) (int64, error) {
	parentCtx := ctx
	pool := p.BufferPool
	if pool == nil {
		pool = NewBufferPool()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stage1 := make(chan []byte, 3)
	stage2 := make(chan []byte, 3)
	bufferSize := pool.SizeForContentType(contentType)

	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
		written  int64
	)

	setErr := func(err error) {
		if err == nil || err == io.EOF {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			cancel()
			if abort != nil {
				_ = abort()
			}
		})
	}

	wg.Add(3)

	go func() {
		defer wg.Done()
		defer close(stage1)

		for {
			select {
			case <-ctx.Done():
				setErr(ctx.Err())
				return
			default:
			}

			buf := pool.Get(bufferSize)
			n, err := src.Read(buf)

			if n > 0 {
				chunk := buf[:n]
				select {
				case stage1 <- chunk:
				case <-ctx.Done():
					pool.Put(chunk)
					setErr(ctx.Err())
					return
				}
			} else {
				pool.Put(buf)
			}

			if err != nil {
				if err != io.EOF {
					setErr(err)
				}
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer close(stage2)

		for {
			select {
			case <-ctx.Done():
				setErr(ctx.Err())
				return
			case chunk, ok := <-stage1:
				if !ok {
					return
				}
				select {
				case stage2 <- chunk:
				case <-ctx.Done():
					pool.Put(chunk)
					return
				}
			}
		}
	}()

	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				setErr(ctx.Err())
				return
			case chunk, ok := <-stage2:
				if !ok {
					return
				}
				n, err := dst.Write(chunk)
				pool.Put(chunk)
				if n > 0 {
					written += int64(n)
				}
				if err != nil {
					setErr(err)
					return
				}
				if n < len(chunk) {
					setErr(io.ErrShortWrite)
					return
				}
			}
		}
	}()

	wg.Wait()

	for chunk := range stage1 {
		pool.Put(chunk)
	}
	for chunk := range stage2 {
		pool.Put(chunk)
	}

	if firstErr != nil {
		return written, firstErr
	}
	if err := parentCtx.Err(); err != nil {
		return written, err
	}

	return written, nil
}
