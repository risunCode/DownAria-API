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
	port := normalizePort(getEnv("PORT", "8080"))
	allowedOrigins := getCSVEnv("ALLOWED_ORIGINS")
	trustedProxyCIDRs := getCSVEnv("TRUSTED_PROXY_CIDRS")
	globalRateLimitLimit, globalRateLimitWindow := parseGlobalRateLimitWindow(
		strings.TrimSpace(getEnv("GLOBAL_RATE_LIMIT_WINDOW", "")),
		60,
		time.Minute,
	)
	globalRateLimitRule := formatGlobalRateLimitRule(globalRateLimitLimit, globalRateLimitWindow)
	globalRateLimitMaxBuckets := getIntEnv("GLOBAL_RATE_LIMIT_MAX_BUCKETS", 10000)
	if globalRateLimitMaxBuckets < 100 {
		globalRateLimitMaxBuckets = 10000
	}
	globalRateLimitBucketTTL := getDurationEnv("GLOBAL_RATE_LIMIT_BUCKET_TTL", 10*time.Minute)

	serverReadTimeout := getDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second)
	serverReadHeaderTimeout := getDurationEnv("SERVER_READ_HEADER_TIMEOUT", 10*time.Second)
	serverWriteTimeout := getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Minute)
	serverIdleTimeout := getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second)
	serverMaxHeaderBytes := getIntEnv("SERVER_MAX_HEADER_BYTES", 1<<20)
	webInternalSharedSecret := strings.TrimSpace(getEnv("WEB_INTERNAL_SHARED_SECRET", ""))
	if serverMaxHeaderBytes < 1024 {
		serverMaxHeaderBytes = 1 << 20
	}

	timeoutMS := getIntEnv("UPSTREAM_TIMEOUT_MS", 10000)
	if timeoutMS < 1 {
		timeoutMS = 10000
	}
	mergeEnabled := getBoolEnv("MERGE_ENABLED", false)
	publicBaseURL := strings.TrimSpace(getEnv("PUBLIC_BASE_URL", ""))
	if publicBaseURL == "" {
		publicBaseURL = fmt.Sprintf("http://localhost:%s", port)
	}

	maxDownloadMB := getIntEnv("MAX_DOWNLOAD_SIZE_MB", 1024)
	if maxDownloadMB < 1 {
		maxDownloadMB = 1024
	}

	maxMergeOutputMB := getIntEnv("MAX_MERGE_OUTPUT_SIZE_MB", 512)
	if maxMergeOutputMB < 1 {
		maxMergeOutputMB = 512
	}

	statsPersistEnabled := getBoolEnv("STATS_PERSIST_ENABLED", false)
	statsPersistFilePath := strings.TrimSpace(getEnv("STATS_PERSIST_FILE_PATH", "./data/public_stats.json"))
	if statsPersistFilePath == "" {
		statsPersistFilePath = "./data/public_stats.json"
	}
	statsPersistFlushIntervalMS := getIntEnv("STATS_PERSIST_FLUSH_INTERVAL_MS", 5000)
	if statsPersistFlushIntervalMS < 1000 {
		statsPersistFlushIntervalMS = 1000
	}
	statsPersistFlushThreshold := getIntEnv("STATS_PERSIST_FLUSH_THRESHOLD", 10)
	if statsPersistFlushThreshold < 1 {
		statsPersistFlushThreshold = 10
	}

	defaultExtractionTTL, defaultProxyHeadTTL, defaultCleanupInterval := CacheDefaults()

	extractionDefaultTTL := getDurationEnv("CACHE_EXTRACTION_TTL", defaultExtractionTTL)
	cacheExtractionPlatformTTLs := map[string]time.Duration{}

	cacheProxyHeadTTL := getDurationEnv("CACHE_PROXY_HEAD_TTL", defaultProxyHeadTTL)
	cacheCleanupInterval := getDurationEnv("CACHE_CLEANUP_INTERVAL", defaultCleanupInterval)

	extractionMaxRetries := getIntEnv("EXTRACTION_MAX_RETRIES", 3)
	if extractionMaxRetries < 1 {
		extractionMaxRetries = 3
	}

	extractionRetryDelayMs := getIntEnv("EXTRACTION_RETRY_DELAY_MS", 500)
	if extractionRetryDelayMs < 0 {
		extractionRetryDelayMs = 500
	}

	return Config{
		Port:                        port,
		AllowedOrigins:              allowedOrigins,
		TrustedProxyCIDRs:           trustedProxyCIDRs,
		GlobalRateLimitLimit:        globalRateLimitLimit,
		GlobalRateLimitWindow:       globalRateLimitWindow,
		GlobalRateLimitRule:         globalRateLimitRule,
		GlobalRateLimitMaxBuckets:   globalRateLimitMaxBuckets,
		GlobalRateLimitBucketTTL:    globalRateLimitBucketTTL,
		UpstreamTimeout:             time.Duration(timeoutMS) * time.Millisecond,
		MergeEnabled:                mergeEnabled,
		PublicBaseURL:               publicBaseURL,
		UpstreamTimeoutMS:           timeoutMS,
		MaxDownloadSizeMB:           maxDownloadMB,
		MaxMergeOutputSizeMB:        maxMergeOutputMB,
		ServerReadTimeout:           serverReadTimeout,
		ServerReadHeaderTimeout:     serverReadHeaderTimeout,
		ServerWriteTimeout:          serverWriteTimeout,
		ServerIdleTimeout:           serverIdleTimeout,
		ServerMaxHeaderBytes:        serverMaxHeaderBytes,
		WebInternalSharedSecret:     webInternalSharedSecret,
		StatsPersistEnabled:         statsPersistEnabled,
		StatsPersistFilePath:        statsPersistFilePath,
		StatsPersistFlushInterval:   time.Duration(statsPersistFlushIntervalMS) * time.Millisecond,
		StatsPersistFlushIntervalMS: statsPersistFlushIntervalMS,
		StatsPersistFlushThreshold:  statsPersistFlushThreshold,
		ExtractionMaxRetries:        extractionMaxRetries,
		ExtractionRetryDelayMs:      extractionRetryDelayMs,
		CacheExtractionTTL:          extractionDefaultTTL,
		CacheExtractionPlatformTTLs: cacheExtractionPlatformTTLs,
		CacheProxyHeadTTL:           cacheProxyHeadTTL,
		CacheCleanupInterval:        cacheCleanupInterval,
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
