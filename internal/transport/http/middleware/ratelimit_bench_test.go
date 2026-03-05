package middleware

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkRateLimiter_Allow(b *testing.B) {
	limiter := NewRateLimiter(100, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("test-ip")
	}
}

func BenchmarkRateLimiter_AllowParallel(b *testing.B) {
	limiter := NewRateLimiter(100, time.Minute)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Simulate different IPs
			ip := "192.168.1." + string(rune(i%255))
			limiter.Allow(ip)
			i++
		}
	})
}

func BenchmarkRateLimiter_AllowWithEviction(b *testing.B) {
	limiter := NewRateLimiter(100, time.Minute)
	limiter.ConfigureBuckets(1000, 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Force eviction by using many different IPs
		ip := "192.168." + string(rune(i/255)) + "." + string(rune(i%255))
		limiter.Allow(ip)
	}
}

func BenchmarkRateLimiter_ConcurrentDifferentIPs(b *testing.B) {
	limiter := NewRateLimiter(100, time.Minute)
	var counter int
	var mu sync.Mutex

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			counter++
			ip := "192.168.1." + string(rune(counter%255))
			mu.Unlock()

			limiter.Allow(ip)
		}
	})
}
