package config

import "time"

type Config struct {
	HTTP       HTTPConfig
	Extraction ExtractionConfig
	Cache      CacheConfig
	Artifacts  ArtifactConfig
	Jobs       JobConfig
	Security   SecurityConfig
	YTDLP      YTDLPConfig
	Logging    LoggingConfig
	Outbound   OutboundConfig
	RateLimit  RateLimitConfig
	Media      MediaConfig
}

// RateLimitConfig controls per-platform upstream request rate limiting.
type RateLimitConfig struct {
	// PerPlatformRPS sets the maximum requests per second for each platform.
	// Platforms not listed use the default of 10 req/sec.
	PerPlatformRPS map[string]int
	MediaRPS       float64
	MediaBurst     int
	JobRPS         float64
	JobBurst       int
}

type HTTPConfig struct {
	Addr                                   string
	ReadTimeout, WriteTimeout, IdleTimeout time.Duration
}

type ExtractionConfig struct{ Timeout time.Duration }

type CacheConfig struct {
	Dir string
	TTL time.Duration
}

type ArtifactConfig struct {
	Dir      string
	TTL      time.Duration
	MaxBytes int64
}

type JobConfig struct {
	Dir     string
	Timeout time.Duration
}

type SecurityConfig struct {
	MaxDownloadBytes int64
	MaxOutputBytes   int64
	MaxDuration      time.Duration
}

type YTDLPConfig struct{ BinaryPath string }

type MediaConfig struct {
	WorkspaceTTL time.Duration
}

type LoggingConfig struct{ Level, Format string }

type OutboundConfig struct {
	BlockedCIDRs []string
}
