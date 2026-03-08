package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func Load() Config {
	// Essential configuration from environment
	port := normalizePort(getEnv("PORT", "8080"))
	allowedOrigins := getCSVEnv("ALLOWED_ORIGINS")
	webInternalSharedSecret := strings.TrimSpace(getEnv("WEB_INTERNAL_SHARED_SECRET", ""))

	// Rate limiting
	globalRateLimitLimit, globalRateLimitWindow := parseGlobalRateLimitWindow(
		strings.TrimSpace(getEnv("GLOBAL_RATE_LIMIT_WINDOW", "")),
		60,
		time.Minute,
	)
	globalRateLimitRule := formatGlobalRateLimitRule(globalRateLimitLimit, globalRateLimitWindow)

	// Upstream timeout
	timeoutMS := getIntEnv("UPSTREAM_TIMEOUT_MS", 10000)
	if timeoutMS < 1 {
		timeoutMS = 10000
	}

	// Download limits
	maxDownloadMB := getIntEnv("MAX_DOWNLOAD_SIZE_MB", 1024)
	if maxDownloadMB < 1 {
		maxDownloadMB = 1024
	}

	// Merge
	mergeEnabled := getBoolEnv("MERGE_ENABLED", false)

	// Stats persistence
	statsPersistEnabled := getBoolEnv("STATS_PERSIST_ENABLED", false)
	statsPersistFilePath := strings.TrimSpace(getEnv("STATS_PERSIST_FILE_PATH", "./data/public_stats.json"))
	if statsPersistFilePath == "" {
		statsPersistFilePath = "./data/public_stats.json"
	}
	statsPersistFlushIntervalMS := getIntEnv("STATS_PERSIST_FLUSH_INTERVAL_MS", 5000)
	if statsPersistFlushIntervalMS < 1000 {
		statsPersistFlushIntervalMS = 1000
	}

	// Cache TTLs
	defaultExtractionTTL, defaultProxyHeadTTL, defaultCleanupInterval := CacheDefaults()
	extractionDefaultTTL := getDurationEnv("CACHE_EXTRACTION_TTL", defaultExtractionTTL)
	cacheProxyHeadTTL := getDurationEnv("CACHE_PROXY_HEAD_TTL", defaultProxyHeadTTL)

	// Auto-generated values
	publicBaseURL := fmt.Sprintf("http://localhost:%s", port)

	// Hardcoded defaults (not exposed via env)
	const (
		serverReadTimeout          = 15 * time.Second
		serverReadHeaderTimeout    = 10 * time.Second
		serverWriteTimeout         = 15 * time.Minute
		serverIdleTimeout          = 60 * time.Second
		serverMaxHeaderBytes       = 1 << 20 // 1MB
		globalRateLimitMaxBuckets  = 10000
		globalRateLimitBucketTTL   = 10 * time.Minute
		maxMergeOutputMB           = 512
		extractionMaxRetries       = 3
		extractionRetryDelayMs     = 500
		statsPersistFlushThreshold = 10
		mergeWorkerCount           = 3
		hlsSegmentWorkerCount      = 5
		hlsSegmentMaxRetries       = 3
		bufferSizeVideo            = 256 * 1024
		bufferSizeAudio            = 64 * 1024
	)

	// Derived upstream timeouts
	upstreamConnectTimeoutMS := timeoutMS
	upstreamTLSHandshakeTimeoutMS := timeoutMS
	upstreamResponseHeaderTimeoutMS := timeoutMS
	upstreamIdleConnTimeoutMS := 90000
	upstreamKeepAliveTimeoutMS := 30000

	return Config{
		Port:                            port,
		AllowedOrigins:                  allowedOrigins,
		TrustedProxyCIDRs:               nil, // Not exposed via env
		GlobalRateLimitLimit:            globalRateLimitLimit,
		GlobalRateLimitWindow:           globalRateLimitWindow,
		GlobalRateLimitRule:             globalRateLimitRule,
		GlobalRateLimitMaxBuckets:       globalRateLimitMaxBuckets,
		GlobalRateLimitBucketTTL:        globalRateLimitBucketTTL,
		UpstreamTimeout:                 time.Duration(timeoutMS) * time.Millisecond,
		UpstreamConnectTimeout:          time.Duration(upstreamConnectTimeoutMS) * time.Millisecond,
		UpstreamTLSHandshakeTimeout:     time.Duration(upstreamTLSHandshakeTimeoutMS) * time.Millisecond,
		UpstreamResponseHeaderTimeout:   time.Duration(upstreamResponseHeaderTimeoutMS) * time.Millisecond,
		UpstreamIdleConnTimeout:         time.Duration(upstreamIdleConnTimeoutMS) * time.Millisecond,
		UpstreamKeepAliveTimeout:        time.Duration(upstreamKeepAliveTimeoutMS) * time.Millisecond,
		MergeEnabled:                    mergeEnabled,
		PublicBaseURL:                   publicBaseURL,
		UpstreamTimeoutMS:               timeoutMS,
		UpstreamConnectTimeoutMS:        upstreamConnectTimeoutMS,
		UpstreamTLSHandshakeTimeoutMS:   upstreamTLSHandshakeTimeoutMS,
		UpstreamResponseHeaderTimeoutMS: upstreamResponseHeaderTimeoutMS,
		UpstreamIdleConnTimeoutMS:       upstreamIdleConnTimeoutMS,
		UpstreamKeepAliveTimeoutMS:      upstreamKeepAliveTimeoutMS,
		MaxDownloadSizeMB:               maxDownloadMB,
		MaxMergeOutputSizeMB:            maxMergeOutputMB,
		ServerReadTimeout:               serverReadTimeout,
		ServerReadHeaderTimeout:         serverReadHeaderTimeout,
		ServerWriteTimeout:              serverWriteTimeout,
		ServerIdleTimeout:               serverIdleTimeout,
		ServerMaxHeaderBytes:            serverMaxHeaderBytes,
		WebInternalSharedSecret:         webInternalSharedSecret,
		StatsPersistEnabled:             statsPersistEnabled,
		StatsPersistFilePath:            statsPersistFilePath,
		StatsPersistFlushInterval:       time.Duration(statsPersistFlushIntervalMS) * time.Millisecond,
		StatsPersistFlushIntervalMS:     statsPersistFlushIntervalMS,
		StatsPersistFlushThreshold:      statsPersistFlushThreshold,
		ExtractionMaxRetries:            extractionMaxRetries,
		ExtractionRetryDelayMs:          extractionRetryDelayMs,
		CacheExtractionTTL:              extractionDefaultTTL,
		CacheExtractionPlatformTTLs:     map[string]time.Duration{},
		CacheProxyHeadTTL:               cacheProxyHeadTTL,
		CacheCleanupInterval:            defaultCleanupInterval,
		StreamingDownloadEnabled:        true,  // Always enabled after optimization
		ConcurrentMergeEnabled:          false, // Not used
		HLSStreamingEnabled:             true,  // Always enabled
		HLSMergeEnabled:                 true,  // Always enabled
		MergeWorkerCount:                mergeWorkerCount,
		HLSSegmentWorkerCount:           hlsSegmentWorkerCount,
		HLSSegmentMaxRetries:            hlsSegmentMaxRetries,
		BufferSizeVideo:                 bufferSizeVideo,
		BufferSizeAudio:                 bufferSizeAudio,
		StreamingDownloadRollout:        100, // Always 100%
		ConcurrentMergeRollout:          100, // Always 100%
		HLSStreamingRollout:             100, // Always 100%
		HLSMergeRollout:                 100, // Always 100%
	}
}

func parseGlobalRateLimitWindow(raw string, fallbackLimit int, fallbackWindow time.Duration) (int, time.Duration) {
	if fallbackLimit < 1 {
		fallbackLimit = 60
	}
	if fallbackWindow <= 0 {
		fallbackWindow = time.Minute
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallbackLimit, fallbackWindow
	}

	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return fallbackLimit, fallbackWindow
	}

	limitPart := strings.TrimSpace(parts[0])
	windowPart := normalizeDurationLiteral(strings.TrimSpace(parts[1]))

	limit, err := strconv.Atoi(limitPart)
	if err != nil || limit < 1 {
		return fallbackLimit, fallbackWindow
	}

	window, err := time.ParseDuration(windowPart)
	if err != nil || window <= 0 {
		return fallbackLimit, fallbackWindow
	}

	return limit, window
}

func formatGlobalRateLimitRule(limit int, window time.Duration) string {
	if limit < 1 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return fmt.Sprintf("%d/%s", limit, normalizeDurationLiteral(window.String()))
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func normalizePort(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "8080"
	}

	if strings.HasPrefix(value, ":") {
		value = strings.TrimPrefix(value, ":")
	}

	if strings.Contains(value, "://") {
		if u, err := url.Parse(value); err == nil {
			if p := u.Port(); p != "" {
				return p
			}
		}
	}

	if strings.HasPrefix(strings.ToLower(value), "tcp/") {
		value = strings.TrimSpace(value[len("tcp/"):])
	}

	if _, p, err := net.SplitHostPort(value); err == nil {
		if p != "" {
			return p
		}
	}

	for _, r := range value {
		if r < '0' || r > '9' {
			return "8080"
		}
	}

	return value
}

func getIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(getEnv(key, "")))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback
	}

	raw = normalizeDurationLiteral(raw)

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func normalizeDurationLiteral(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return value
	}

	replacements := []struct {
		suffix string
		repl   string
	}{
		{suffix: "minutes", repl: "m"},
		{suffix: "minute", repl: "m"},
		{suffix: "mins", repl: "m"},
		{suffix: "min", repl: "m"},
		{suffix: "hours", repl: "h"},
		{suffix: "hour", repl: "h"},
		{suffix: "hrs", repl: "h"},
		{suffix: "hr", repl: "h"},
		{suffix: "seconds", repl: "s"},
		{suffix: "second", repl: "s"},
		{suffix: "secs", repl: "s"},
		{suffix: "sec", repl: "s"},
	}

	for _, candidate := range replacements {
		if strings.HasSuffix(normalized, candidate.suffix) {
			base := strings.TrimSpace(strings.TrimSuffix(normalized, candidate.suffix))
			if base != "" {
				return base + candidate.repl
			}
		}
	}

	return normalized
}

func getCSVEnv(key string) []string {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
