package config

import (
	"testing"
	"time"
)

func TestLoad_CacheExtractionTTL_Defaults(t *testing.T) {
	t.Setenv("CACHE_EXTRACTION_TTL", "")

	cfg := Load()

	if cfg.CacheExtractionTTL != 5*time.Minute {
		t.Fatalf("expected default extraction ttl=5m, got %s", cfg.CacheExtractionTTL)
	}

	if len(cfg.CacheExtractionPlatformTTLs) != 0 {
		t.Fatalf("expected no platform-specific extraction TTL overrides, got %d", len(cfg.CacheExtractionPlatformTTLs))
	}
}

func TestLoad_CacheExtractionTTL_Overrides(t *testing.T) {
	t.Setenv("CACHE_EXTRACTION_TTL", "5m")

	cfg := Load()

	if cfg.CacheExtractionTTL != 5*time.Minute {
		t.Fatalf("expected extraction ttl=5m, got %s", cfg.CacheExtractionTTL)
	}
}

func TestLoad_GlobalRateLimit_Defaults(t *testing.T) {
	t.Setenv("GLOBAL_RATE_LIMIT_WINDOW", "")

	cfg := Load()

	if cfg.GlobalRateLimitLimit != 60 {
		t.Fatalf("expected default global rate limit=60, got %d", cfg.GlobalRateLimitLimit)
	}

	if cfg.GlobalRateLimitWindow != time.Minute {
		t.Fatalf("expected global rate limit window=1m, got %s", cfg.GlobalRateLimitWindow)
	}

	if cfg.GlobalRateLimitRule != "60/1m0s" {
		t.Fatalf("expected global rate limit rule=60/1m0s, got %q", cfg.GlobalRateLimitRule)
	}
}

func TestLoad_GlobalRateLimit_Overrides(t *testing.T) {
	t.Setenv("GLOBAL_RATE_LIMIT_WINDOW", "200/4min")

	cfg := Load()

	if cfg.GlobalRateLimitLimit != 200 {
		t.Fatalf("expected global rate limit=200, got %d", cfg.GlobalRateLimitLimit)
	}

	if cfg.GlobalRateLimitWindow != 4*time.Minute {
		t.Fatalf("expected global rate limit window=4m from 4min literal, got %s", cfg.GlobalRateLimitWindow)
	}

	if cfg.GlobalRateLimitRule != "200/4m0s" {
		t.Fatalf("expected global rate limit rule=200/4m0s, got %q", cfg.GlobalRateLimitRule)
	}
}

func TestLoad_TrustedProxyCIDRs(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.1.10,::1")

	cfg := Load()

	if len(cfg.TrustedProxyCIDRs) != 3 {
		t.Fatalf("expected 3 trusted proxies, got %d", len(cfg.TrustedProxyCIDRs))
	}
}

func TestLoad_ServerHardeningDefaults(t *testing.T) {
	t.Setenv("SERVER_READ_TIMEOUT", "")
	t.Setenv("SERVER_READ_HEADER_TIMEOUT", "")
	t.Setenv("SERVER_WRITE_TIMEOUT", "")
	t.Setenv("SERVER_IDLE_TIMEOUT", "")
	t.Setenv("SERVER_MAX_HEADER_BYTES", "")

	cfg := Load()

	if cfg.ServerReadTimeout != 15*time.Second {
		t.Fatalf("expected read timeout=15s, got %s", cfg.ServerReadTimeout)
	}
	if cfg.ServerReadHeaderTimeout != 10*time.Second {
		t.Fatalf("expected read header timeout=10s, got %s", cfg.ServerReadHeaderTimeout)
	}
	if cfg.ServerWriteTimeout != 15*time.Minute {
		t.Fatalf("expected write timeout=15m, got %s", cfg.ServerWriteTimeout)
	}
	if cfg.ServerIdleTimeout != 60*time.Second {
		t.Fatalf("expected idle timeout=60s, got %s", cfg.ServerIdleTimeout)
	}
	if cfg.ServerMaxHeaderBytes != 1<<20 {
		t.Fatalf("expected max header bytes=1MiB, got %d", cfg.ServerMaxHeaderBytes)
	}
}
