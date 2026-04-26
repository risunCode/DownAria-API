package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func LoadFromEnv() (Config, error) {
	httpCfg, err := loadHTTPConfig()
	if err != nil {
		return Config{}, err
	}

	extractTimeout, err := durationEnvOrDefault("EXTRACT_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	cacheTTL, err := durationEnvOrDefault("EXTRACT_CACHE_TTL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}

	artifactTTL, err := durationEnvOrDefault("ARTIFACT_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	artifactMaxBytes, err := bytesEnvOrDefault("ARTIFACT_MAX_BYTES", 2<<30)
	if err != nil {
		return Config{}, err
	}

	jobTimeout, err := durationEnvOrDefault("JOB_TIMEOUT", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	maxDownloadBytes, err := bytesEnvOrDefault("MAX_DOWNLOAD_BYTES", 892<<20)
	if err != nil {
		return Config{}, err
	}

	maxOutputBytes, err := bytesEnvOrDefault("MAX_OUTPUT_BYTES", 892<<20)
	if err != nil {
		return Config{}, err
	}

	maxMediaDuration, err := durationEnvOrDefault("MAX_MEDIA_DURATION", 119*time.Minute)
	if err != nil {
		return Config{}, err
	}

	mediaRateLimit, err := intEnvOrDefault("MEDIA_RATE_LIMIT", 20)
	if err != nil {
		return Config{}, err
	}
	mediaRateBurst, err := intEnvOrDefault("MEDIA_RATE_BURST", 20)
	if err != nil {
		return Config{}, err
	}
	jobRateLimit, err := intEnvOrDefault("JOB_RATE_LIMIT", 50)
	if err != nil {
		return Config{}, err
	}
	jobRateBurst, err := intEnvOrDefault("JOB_RATE_BURST", 50)
	if err != nil {
		return Config{}, err
	}
	workspaceTTL, err := durationEnvOrDefault("WORKSPACE_TTL", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTP: httpCfg,
		Extraction: ExtractionConfig{
			Timeout: extractTimeout,
		},
		Cache: CacheConfig{
			Dir: strings.TrimSpace(os.Getenv("EXTRACT_CACHE_DIR")),
			TTL: cacheTTL,
		},
		Artifacts: ArtifactConfig{
			Dir:      strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
			TTL:      artifactTTL,
			MaxBytes: artifactMaxBytes,
		},
		Jobs: JobConfig{
			Dir:     strings.TrimSpace(os.Getenv("JOB_DIR")),
			Timeout: jobTimeout,
		},
		Security: SecurityConfig{
			MaxDownloadBytes: maxDownloadBytes,
			MaxOutputBytes:   maxOutputBytes,
			MaxDuration:      maxMediaDuration,
		},
		YTDLP: YTDLPConfig{
			BinaryPath: stringEnvOrDefault("YTDLP_BINARY", "yt-dlp"),
		},
		Logging: LoggingConfig{
			Level:  strings.ToLower(strings.TrimSpace(stringEnvOrDefault("LOG_LEVEL", "info"))),
			Format: strings.ToLower(strings.TrimSpace(stringEnvOrDefault("LOG_FORMAT", "json"))),
		},
		Outbound: OutboundConfig{
			BlockedCIDRs: csvEnv("OUTBOUND_BLOCKED_CIDRS"),
		},
		RateLimit: RateLimitConfig{
			MediaRPS:   float64(mediaRateLimit) / 60.0,
			MediaBurst: mediaRateBurst,
			JobRPS:     float64(jobRateLimit) / 60.0,
			JobBurst:   jobRateBurst,
		},
		Media: MediaConfig{
			WorkspaceTTL: workspaceTTL,
		},
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadHTTPConfig() (HTTPConfig, error) {
	readTimeout, err := durationEnvOrDefault("HTTP_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return HTTPConfig{}, err
	}

	writeTimeout, err := durationEnvOrDefault("HTTP_WRITE_TIMEOUT", 5*time.Minute)
	if err != nil {
		return HTTPConfig{}, err
	}

	idleTimeout, err := durationEnvOrDefault("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return HTTPConfig{}, err
	}

	return HTTPConfig{
		Addr:         loadListenAddr(),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}, nil
}

func stringEnvOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func loadListenAddr() string {
	if addr := strings.TrimSpace(os.Getenv("ADDR")); addr != "" {
		return addr
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if strings.Contains(port, ":") {
			return port
		}
		return ":" + port
	}
	return ":8080"
}

func durationEnvOrDefault(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return parsed, nil
}

func intEnvOrDefault(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}
	return parsed, nil
}

func csvEnv(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func bytesEnvOrDefault(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	var result int64
	_, err := fmt.Sscan(value, &result)
	if err != nil {
		return 0, fmt.Errorf("invalid bytes value for %s: %w", key, err)
	}
	if result <= 0 {
		return 0, fmt.Errorf("invalid bytes value for %s: must be > 0", key)
	}
	return result, nil
}
