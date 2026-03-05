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
