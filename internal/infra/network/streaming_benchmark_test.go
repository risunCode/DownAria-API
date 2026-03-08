package network

import (
	"context"
	"io"
	"runtime"
	"testing"
	"time"
)

// mockReadCloser simulates a data source with configurable size
type mockReadCloser struct {
	size      int64
	remaining int64
	chunkSize int
}

func newMockReadCloser(size int64) *mockReadCloser {
	return &mockReadCloser{
		size:      size,
		remaining: size,
		chunkSize: 32 * 1024, // 32KB chunks
	}
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.remaining <= 0 {
		return 0, io.EOF
	}

	toRead := int64(len(p))
	if toRead > m.remaining {
		toRead = m.remaining
	}
	if toRead > int64(m.chunkSize) {
		toRead = int64(m.chunkSize)
	}

	// Simulate some data
	for i := int64(0); i < toRead; i++ {
		p[i] = byte(i % 256)
	}

	m.remaining -= toRead
	return int(toRead), nil
}

func (m *mockReadCloser) Close() error {
	return nil
}

// discardWriter is a writer that discards all data (like /dev/null)
type discardWriter struct {
	written int64
}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	d.written += int64(n)
	return n, nil
}

// BenchmarkStreamingDownloader_SmallFile benchmarks streaming with 1MB file
func BenchmarkStreamingDownloader_SmallFile(b *testing.B) {
	benchmarkStreamingDownloader(b, 1*1024*1024) // 1MB
}

// BenchmarkStreamingDownloader_MediumFile benchmarks streaming with 50MB file
func BenchmarkStreamingDownloader_MediumFile(b *testing.B) {
	benchmarkStreamingDownloader(b, 50*1024*1024) // 50MB
}

// BenchmarkStreamingDownloader_LargeFile benchmarks streaming with 500MB file
func BenchmarkStreamingDownloader_LargeFile(b *testing.B) {
	benchmarkStreamingDownloader(b, 500*1024*1024) // 500MB
}

func benchmarkStreamingDownloader(b *testing.B, fileSize int64) {
	pool := NewBufferPool()
	ctx := context.Background()

	b.SetBytes(fileSize)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		src := newMockReadCloser(fileSize)
		dst := &discardWriter{}

		downloader := NewStreamingDownloader(pool)
		_, err := downloader.StreamWithBuffer(ctx, src, dst, "video/mp4", fileSize)
		if err != nil {
			b.Fatalf("streaming failed: %v", err)
		}
	}
}

// BenchmarkConcurrentDownloads benchmarks concurrent downloads
func BenchmarkConcurrentDownloads(b *testing.B) {
	benchmarkConcurrentDownloads(b, 10, 10*1024*1024) // 10 concurrent, 10MB each
}

func benchmarkConcurrentDownloads(b *testing.B, concurrency int, fileSize int64) {
	pool := NewBufferPool()
	ctx := context.Background()

	b.SetBytes(fileSize * int64(concurrency))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan error, concurrency)

		for j := 0; j < concurrency; j++ {
			go func() {
				src := newMockReadCloser(fileSize)
				dst := &discardWriter{}

				downloader := NewStreamingDownloader(pool)
				_, err := downloader.StreamWithBuffer(ctx, src, dst, "video/mp4", fileSize)
				done <- err
			}()
		}

		// Wait for all downloads to complete
		for j := 0; j < concurrency; j++ {
			if err := <-done; err != nil {
				b.Fatalf("Concurrent download failed: %v", err)
			}
		}
	}
}

// BenchmarkMemoryUsage measures memory usage
func BenchmarkMemoryUsage(b *testing.B) {
	benchmarkMemoryUsage(b)
}

func benchmarkMemoryUsage(b *testing.B) {
	pool := NewBufferPool()
	ctx := context.Background()
	fileSize := int64(100 * 1024 * 1024) // 100MB

	b.ReportAllocs()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := newMockReadCloser(fileSize)
		dst := &discardWriter{}

		downloader := NewStreamingDownloader(pool)
		_, _ = downloader.StreamWithBuffer(ctx, src, dst, "video/mp4", fileSize)
	}
	b.StopTimer()

	runtime.GC()
	runtime.ReadMemStats(&m2)

	b.ReportMetric(float64(m2.TotalAlloc-m1.TotalAlloc)/float64(b.N), "B/op")
	b.ReportMetric(float64(m2.Mallocs-m1.Mallocs)/float64(b.N), "allocs/op")
}

// BenchmarkGoroutineCount measures goroutine overhead
func BenchmarkGoroutineCount(b *testing.B) {
	benchmarkGoroutineCount(b)
}

func benchmarkGoroutineCount(b *testing.B) {
	pool := NewBufferPool()
	ctx := context.Background()
	fileSize := int64(10 * 1024 * 1024) // 10MB

	initialGoroutines := runtime.NumGoroutine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := newMockReadCloser(fileSize)
		dst := &discardWriter{}

		downloader := NewStreamingDownloader(pool)
		_, _ = downloader.StreamWithBuffer(ctx, src, dst, "video/mp4", fileSize)
	}
	b.StopTimer()

	// Allow goroutines to finish
	time.Sleep(100 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	b.ReportMetric(float64(finalGoroutines-initialGoroutines), "goroutines")
}

// BenchmarkBufferPoolAdaptive tests adaptive buffer sizing
func BenchmarkBufferPoolAdaptive(b *testing.B) {
	pool := NewBufferPool()

	scenarios := []struct {
		name          string
		contentType   string
		contentLength int64
	}{
		{"SmallImage", "image/jpeg", 500 * 1024},           // 500KB
		{"MediumVideo", "video/mp4", 50 * 1024 * 1024},     // 50MB
		{"LargeVideo", "video/mp4", 500 * 1024 * 1024},     // 500MB
		{"Audio", "audio/mpeg", 5 * 1024 * 1024},           // 5MB
		{"UnknownSmall", "application/octet-stream", 1024}, // 1KB
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				size := pool.OptimalSizeForContentType(sc.contentType, sc.contentLength)
				buf := pool.Get(size)
				pool.Put(buf)
			}
		})
	}
}

// BenchmarkThroughput measures throughput in MB/s
func BenchmarkThroughput(b *testing.B) {
	benchmarkThroughput(b)
}

func benchmarkThroughput(b *testing.B) {
	pool := NewBufferPool()
	ctx := context.Background()
	fileSize := int64(100 * 1024 * 1024) // 100MB

	b.SetBytes(fileSize)
	b.ResetTimer()

	start := time.Now()
	var totalBytes int64

	for i := 0; i < b.N; i++ {
		src := newMockReadCloser(fileSize)
		dst := &discardWriter{}

		downloader := NewStreamingDownloader(pool)
		written, _ := downloader.StreamWithBuffer(ctx, src, dst, "video/mp4", fileSize)
		totalBytes += written
	}

	elapsed := time.Since(start)
	throughputMBps := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	b.ReportMetric(throughputMBps, "MB/s")
}
