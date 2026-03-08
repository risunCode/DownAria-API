package network

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	VideoBufferSize   = 256 * 1024
	AudioBufferSize   = 64 * 1024
	ImageBufferSize   = 32 * 1024
	DefaultBufferSize = 128 * 1024
)

type BufferPool struct {
	pools          sync.Map
	memoryPressure atomic.Bool
}

func NewBufferPool() *BufferPool {
	return &BufferPool{}
}

func (bp *BufferPool) SetMemoryPressure(v bool) {
	bp.memoryPressure.Store(v)
}

func (bp *BufferPool) DetectMemoryPressure() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Sys == 0 {
		bp.memoryPressure.Store(false)
		return false
	}
	pressure := float64(m.HeapInuse)/float64(m.Sys) > 0.85
	bp.memoryPressure.Store(pressure)
	return pressure
}

func (bp *BufferPool) SizeForContentType(contentType string) int {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	size := DefaultBufferSize
	switch {
	case strings.HasPrefix(ct, "video/"):
		size = VideoBufferSize
	case strings.HasPrefix(ct, "audio/"):
		size = AudioBufferSize
	case strings.HasPrefix(ct, "image/"):
		size = ImageBufferSize
	}
	if bp.memoryPressure.Load() {
		return size / 2
	}
	return size
}

// OptimalSizeForContentType returns optimal buffer size based on content type and content length
// This provides better performance by adapting buffer size to file size
func (bp *BufferPool) OptimalSizeForContentType(contentType string, contentLength int64) int {
	baseSize := bp.SizeForContentType(contentType)

	// For very large files (>100MB), use larger buffers for better throughput
	if contentLength > 100*1024*1024 {
		// Double the buffer size, but cap at 512KB
		optimized := baseSize * 2
		if optimized > 512*1024 {
			optimized = 512 * 1024
		}
		return optimized
	}

	// For small files (<1MB), use smaller buffers to reduce memory waste
	if contentLength > 0 && contentLength < 1024*1024 {
		return 32 * 1024 // 32KB for small files
	}

	// For medium files or unknown size, use base size
	return baseSize
}

func (bp *BufferPool) Get(size int) []byte {
	if size <= 0 {
		size = DefaultBufferSize
	}
	poolAny, _ := bp.pools.LoadOrStore(size, &sync.Pool{
		New: func() any {
			buf := make([]byte, size)
			return &buf
		},
	})
	pool := poolAny.(*sync.Pool)
	bufPtr := pool.Get().(*[]byte)
	buf := *bufPtr
	if cap(buf) < size {
		buf = make([]byte, size)
	}
	return buf[:size]
}

func (bp *BufferPool) Put(buf []byte) {
	if len(buf) == 0 {
		return
	}
	size := cap(buf)
	poolAny, ok := bp.pools.Load(size)
	if !ok {
		poolAny, _ = bp.pools.LoadOrStore(size, &sync.Pool{New: func() any {
			b := make([]byte, size)
			return &b
		}})
	}
	b := buf[:size]
	poolAny.(*sync.Pool).Put(&b)
}
