package config

import (
	"fmt"
	"strings"
	"time"
)

func validate(cfg Config) error {
	if strings.TrimSpace(cfg.HTTP.Addr) == "" {
		return fmt.Errorf("HTTP.Addr must not be empty")
	}
	if cfg.HTTP.ReadTimeout <= 0 {
		return fmt.Errorf("HTTP.ReadTimeout must be > 0")
	}
	if cfg.HTTP.WriteTimeout <= 0 {
		return fmt.Errorf("HTTP.WriteTimeout must be > 0")
	}
	if cfg.HTTP.IdleTimeout <= 0 {
		return fmt.Errorf("HTTP.IdleTimeout must be > 0")
	}
	if cfg.Extraction.Timeout <= 0 || cfg.Extraction.Timeout >= 5*time.Minute {
		return fmt.Errorf("Extraction.Timeout must be > 0 and < 5 minutes")
	}
	if cfg.Cache.TTL <= 0 {
		return fmt.Errorf("Cache.TTL must be > 0")
	}
	if cfg.Artifacts.TTL <= 0 {
		return fmt.Errorf("Artifacts.TTL must be > 0")
	}
	if cfg.Artifacts.MaxBytes != 0 && cfg.Artifacts.MaxBytes <= 0 {
		return fmt.Errorf("Artifacts.MaxBytes must be > 0 if set")
	}
	if cfg.Security.MaxDownloadBytes != 0 && cfg.Security.MaxDownloadBytes <= 0 {
		return fmt.Errorf("Security.MaxDownloadBytes must be > 0 if set")
	}
	if cfg.Security.MaxDuration <= 0 || cfg.Security.MaxDuration >= 2*time.Hour {
		return fmt.Errorf("Security.MaxDuration must be > 0 and < 2 hours")
	}
	if cfg.Security.MaxOutputBytes <= 0 {
		return fmt.Errorf("Security.MaxOutputBytes must be > 0")
	}
	if strings.TrimSpace(cfg.YTDLP.BinaryPath) == "" {
		return fmt.Errorf("YTDLP.BinaryPath must not be empty")
	}
	if cfg.Logging.Level != "debug" && cfg.Logging.Level != "info" && cfg.Logging.Level != "warn" && cfg.Logging.Level != "error" {
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
	if cfg.Logging.Format != "json" && cfg.Logging.Format != "text" && cfg.Logging.Format != "pretty" && cfg.Logging.Format != "auto" {
		return fmt.Errorf("LOG_FORMAT must be one of json, text, pretty, auto")
	}
	if cfg.RateLimit.MediaRPS < 0 || cfg.RateLimit.MediaBurst < 0 {
		return fmt.Errorf("invalid media rate limit config")
	}
	if cfg.RateLimit.JobRPS < 0 || cfg.RateLimit.JobBurst < 0 {
		return fmt.Errorf("invalid job rate limit config")
	}
	if cfg.Media.WorkspaceTTL <= 0 {
		return fmt.Errorf("Media.WorkspaceTTL must be > 0")
	}
	return nil
}
