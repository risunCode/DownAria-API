package cache

import (
	"strings"
	"time"
)

const defaultExtractionTTL = time.Minute

type PlatformTTLConfig struct {
	DefaultTTL   time.Duration
	PlatformTTLs map[string]time.Duration
}

func NewPlatformTTLConfig(defaultTTL time.Duration, platformTTLs map[string]time.Duration) PlatformTTLConfig {
	if defaultTTL <= 0 {
		defaultTTL = defaultExtractionTTL
	}

	cloned := make(map[string]time.Duration, len(platformTTLs))
	for platform, ttl := range platformTTLs {
		normalized := normalizePlatform(platform)
		if normalized == "" || ttl <= 0 {
			continue
		}
		cloned[normalized] = ttl
	}

	return PlatformTTLConfig{
		DefaultTTL:   defaultTTL,
		PlatformTTLs: cloned,
	}
}

func (c PlatformTTLConfig) TTLForPlatform(platform string) time.Duration {
	normalized := normalizePlatform(platform)
	if normalized != "" {
		if ttl, ok := c.PlatformTTLs[normalized]; ok && ttl > 0 {
			return ttl
		}
	}

	if c.DefaultTTL <= 0 {
		return defaultExtractionTTL
	}

	return c.DefaultTTL
}

func normalizePlatform(platform string) string {
	return strings.ToLower(strings.TrimSpace(platform))
}
