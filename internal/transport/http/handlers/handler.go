package handlers

import (
	"net/http"
	"time"

	"downaria-api/internal/app/services/extraction"
	"downaria-api/internal/core/config"
	"downaria-api/internal/core/ports"
	"downaria-api/internal/extractors"
	"downaria-api/internal/extractors/registry"
	"downaria-api/internal/infra/cache"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/infra/persistence"
	"downaria-api/internal/shared/security"
	"downaria-api/internal/shared/util"
)

type Handler struct {
	config     config.Config
	startedAt  time.Time
	httpClient *http.Client
	statsStore ports.StatsStore
	Streamer   *network.Streamer
	extractor  extraction.Service
	headCache  *cache.TTLCache
	clientIPFn func(*http.Request) string
	urlGuard   *security.OutboundURLValidator
}

type statsStoreCloser interface {
	Close() error
}

func NewHandler(cfg config.Config, startedAt time.Time) *Handler {
	reg := registry.NewRegistry()

	extractors.RegisterDefaultExtractors(reg)

	baseExtractor := extraction.NewService(reg, 30, cfg.ExtractionMaxRetries, cfg.ExtractionRetryDelayMs)
	cachedExtractor := extraction.NewCachedService(baseExtractor, cache.NewPlatformTTLConfig(cfg.CacheExtractionTTL, cfg.CacheExtractionPlatformTTLs))
	trustedProxies, err := util.NewIPAllowlist(cfg.TrustedProxyCIDRs)
	if err != nil {
		trustedProxies = nil
	}

	return &Handler{
		config:     cfg,
		startedAt:  startedAt,
		httpClient: network.GetClientWithTimeout(cfg.UpstreamTimeout),
		statsStore: persistence.NewPublicStatsStore(startedAt, persistence.PublicStatsPersistenceOptions{
			Enabled:        cfg.StatsPersistEnabled,
			FilePath:       cfg.StatsPersistFilePath,
			FlushInterval:  cfg.StatsPersistFlushInterval,
			FlushThreshold: cfg.StatsPersistFlushThreshold,
		}),
		Streamer:  network.NewStreamer(),
		extractor: cachedExtractor,
		headCache: cache.NewTTLCacheWithMaxEntries(2048),
		clientIPFn: func(r *http.Request) string {
			return util.ClientIPFromRequestWithTrustedProxies(r, trustedProxies)
		},
		urlGuard: security.NewOutboundURLValidator(nil),
	}
}

func (h *Handler) Close() error {
	closer, ok := h.statsStore.(statsStoreCloser)
	if !ok {
		return nil
	}
	return closer.Close()
}
