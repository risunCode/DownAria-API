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
	// TrustedProxyCIDRs is no longer configurable via env (hardcoded to nil)
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.1.10,::1")

	cfg := Load()

	if len(cfg.TrustedProxyCIDRs) != 0 {
		t.Fatalf("expected 0 trusted proxies (hardcoded), got %d", len(cfg.TrustedProxyCIDRs))
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

func TestLoad_ContentDeliveryFlags(t *testing.T) {
	// Feature flags are now hardcoded (always enabled after optimization)
	t.Setenv("STREAMING_DOWNLOAD_ENABLED", "false")
	t.Setenv("CONCURRENT_MERGE_ENABLED", "false")
	t.Setenv("HLS_STREAMING_ENABLED", "false")
	t.Setenv("HLS_MERGE_ENABLED", "false")
	t.Setenv("MERGE_WORKER_COUNT", "4")
	t.Setenv("HLS_SEGMENT_WORKER_COUNT", "8")
	t.Setenv("HLS_SEGMENT_MAX_RETRIES", "2")
	t.Setenv("BUFFER_SIZE_VIDEO", "262144")
	t.Setenv("BUFFER_SIZE_AUDIO", "65536")

	cfg := Load()

	// Feature flags are hardcoded to true
	if !cfg.StreamingDownloadEnabled || !cfg.HLSStreamingEnabled || !cfg.HLSMergeEnabled {
		t.Fatalf("expected all streaming features enabled (hardcoded)")
	}

	// Worker counts use hardcoded defaults (env vars ignored)
	if cfg.MergeWorkerCount != 3 || cfg.HLSSegmentWorkerCount != 5 || cfg.HLSSegmentMaxRetries != 3 {
		t.Fatalf("expected hardcoded worker settings (3, 5, 3), got (%d, %d, %d)",
			cfg.MergeWorkerCount, cfg.HLSSegmentWorkerCount, cfg.HLSSegmentMaxRetries)
	}
}

func TestLoad_UpstreamTransportTimeouts_DefaultFromUpstreamTimeout(t *testing.T) {
	t.Setenv("UPSTREAM_TIMEOUT_MS", "7000")
	t.Setenv("UPSTREAM_CONNECT_TIMEOUT_MS", "")
	t.Setenv("UPSTREAM_TLS_HANDSHAKE_TIMEOUT_MS", "")
	t.Setenv("UPSTREAM_RESPONSE_HEADER_TIMEOUT_MS", "")
	t.Setenv("UPSTREAM_IDLE_CONN_TIMEOUT_MS", "")
	t.Setenv("UPSTREAM_KEEPALIVE_TIMEOUT_MS", "")

	cfg := Load()

	if cfg.UpstreamTimeout != 7*time.Second {
		t.Fatalf("expected upstream timeout=7s, got %s", cfg.UpstreamTimeout)
	}
	if cfg.UpstreamConnectTimeout != 7*time.Second {
		t.Fatalf("expected connect timeout=7s, got %s", cfg.UpstreamConnectTimeout)
	}
	if cfg.UpstreamTLSHandshakeTimeout != 7*time.Second {
		t.Fatalf("expected tls handshake timeout=7s, got %s", cfg.UpstreamTLSHandshakeTimeout)
	}
	if cfg.UpstreamResponseHeaderTimeout != 7*time.Second {
		t.Fatalf("expected response header timeout=7s, got %s", cfg.UpstreamResponseHeaderTimeout)
	}
	if cfg.UpstreamIdleConnTimeout != 90*time.Second {
		t.Fatalf("expected idle conn timeout=90s, got %s", cfg.UpstreamIdleConnTimeout)
	}
	if cfg.UpstreamKeepAliveTimeout != 30*time.Second {
		t.Fatalf("expected keepalive timeout=30s, got %s", cfg.UpstreamKeepAliveTimeout)
	}
}

func TestLoad_UpstreamTransportTimeouts_Overrides(t *testing.T) {
	// Individual timeout overrides are no longer supported (hardcoded to derive from UPSTREAM_TIMEOUT_MS)
	t.Setenv("UPSTREAM_TIMEOUT_MS", "10000")
	t.Setenv("UPSTREAM_CONNECT_TIMEOUT_MS", "1500")
	t.Setenv("UPSTREAM_TLS_HANDSHAKE_TIMEOUT_MS", "2000")
	t.Setenv("UPSTREAM_RESPONSE_HEADER_TIMEOUT_MS", "2500")
	t.Setenv("UPSTREAM_IDLE_CONN_TIMEOUT_MS", "3000")
	t.Setenv("UPSTREAM_KEEPALIVE_TIMEOUT_MS", "3500")

	cfg := Load()

	// All timeouts now derive from UPSTREAM_TIMEOUT_MS (hardcoded)
	if cfg.UpstreamConnectTimeoutMS != 10000 {
		t.Fatalf("expected connect timeout ms=10000 (derived from UPSTREAM_TIMEOUT_MS), got %d", cfg.UpstreamConnectTimeoutMS)
	}
	if cfg.UpstreamTLSHandshakeTimeoutMS != 10000 {
		t.Fatalf("expected tls handshake timeout ms=10000 (derived from UPSTREAM_TIMEOUT_MS), got %d", cfg.UpstreamTLSHandshakeTimeoutMS)
	}
	if cfg.UpstreamResponseHeaderTimeoutMS != 10000 {
		t.Fatalf("expected response header timeout ms=10000 (derived from UPSTREAM_TIMEOUT_MS), got %d", cfg.UpstreamResponseHeaderTimeoutMS)
	}
	// Idle and keepalive use hardcoded defaults
	if cfg.UpstreamIdleConnTimeoutMS != 90000 {
		t.Fatalf("expected idle conn timeout ms=90000 (hardcoded), got %d", cfg.UpstreamIdleConnTimeoutMS)
	}
	if cfg.UpstreamKeepAliveTimeoutMS != 30000 {
		t.Fatalf("expected keepalive timeout ms=30000 (hardcoded), got %d", cfg.UpstreamKeepAliveTimeoutMS)
	}
}
