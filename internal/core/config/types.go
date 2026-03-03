package config

import "time"

type Config struct {
	Port                        string
	AllowedOrigins              []string
	TrustedProxyCIDRs           []string
	GlobalRateLimitLimit        int
	GlobalRateLimitWindow       time.Duration
	GlobalRateLimitRule         string
	GlobalRateLimitMaxBuckets   int
	GlobalRateLimitBucketTTL    time.Duration
	UpstreamTimeout             time.Duration
	MergeEnabled                bool
	PublicBaseURL               string
	UpstreamTimeoutMS           int
	MaxDownloadSizeMB           int
	MaxMergeOutputSizeMB        int
	ServerReadTimeout           time.Duration
	ServerReadHeaderTimeout     time.Duration
	ServerWriteTimeout          time.Duration
	ServerIdleTimeout           time.Duration
	ServerMaxHeaderBytes        int
	WebInternalSharedSecret     string
	StatsPersistEnabled         bool
	StatsPersistFilePath        string
	StatsPersistFlushInterval   time.Duration
	StatsPersistFlushIntervalMS int
	StatsPersistFlushThreshold  int

	ExtractionMaxRetries   int
	ExtractionRetryDelayMs int

	// Cache TTL configurations
	CacheExtractionTTL          time.Duration            // Default TTL for extraction results (default: 5m)
	CacheExtractionPlatformTTLs map[string]time.Duration // Platform-specific extraction TTLs
	CacheProxyHeadTTL           time.Duration            // TTL for proxy HEAD metadata (default: 45s)
	CacheCleanupInterval        time.Duration            // Interval for cache cleanup (default: 5m)

	// Performance profiling
	EnableProfiling bool   // Enable pprof endpoints
	ProfilingPort   string // Port for profiling server (default: 6060)
	EnableMetrics   bool   // Enable Prometheus metrics endpoint
}

// CacheDefaults returns default cache TTL values
func CacheDefaults() (extractionTTL, proxyHeadTTL, cleanupInterval time.Duration) {
	return 5 * time.Minute, 45 * time.Second, 5 * time.Minute
}

// CacheExtractionPlatformDefaults returns default extraction TTLs per platform.
func CacheExtractionPlatformDefaults() map[string]time.Duration {
	return map[string]time.Duration{
		"instagram": 5 * time.Minute,
		"twitter":   2 * time.Minute,
		"tiktok":    3 * time.Minute,
		"facebook":  5 * time.Minute,
		"pixiv":     5 * time.Minute,
		"youtube":   10 * time.Minute,
	}
}

// IsCacheEnabled returns true if any caching is enabled
func (c Config) IsCacheEnabled() bool {
	return c.CacheExtractionTTL > 0 || c.CacheProxyHeadTTL > 0
}
