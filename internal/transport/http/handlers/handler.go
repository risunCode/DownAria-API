package handlers

import (
	"net/http"
	"time"

	"fetchmoona/internal/app/services/extraction"
	"fetchmoona/internal/core/config"
	"fetchmoona/internal/core/ports"
	"fetchmoona/internal/extractors"
	"fetchmoona/internal/extractors/registry"
	"fetchmoona/internal/infra/cache"
	"fetchmoona/internal/infra/network"
	"fetchmoona/internal/infra/persistence"
	"fetchmoona/internal/shared/security"
	"fetchmoona/internal/shared/util"
	"golang.org/x/sync/singleflight"
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
	headGroup  singleflight.Group
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
	urlGuard := security.NewOutboundURLValidator(nil)
	guardedClient := network.GetClientWithTimeoutGuard(cfg.UpstreamTimeout, urlGuard)

	return &Handler{
		config:     cfg,
		startedAt:  startedAt,
		httpClient: guardedClient,
		statsStore: persistence.NewPublicStatsStore(startedAt, persistence.PublicStatsPersistenceOptions{
			Enabled:        cfg.StatsPersistEnabled,
			FilePath:       cfg.StatsPersistFilePath,
			FlushInterval:  cfg.StatsPersistFlushInterval,
			FlushThreshold: cfg.StatsPersistFlushThreshold,
		}),
		Streamer:  network.NewStreamerWithClient(guardedClient),
		extractor: cachedExtractor,
		headCache: cache.NewTTLCacheWithMaxEntries(2048),
		clientIPFn: func(r *http.Request) string {
			return util.ClientIPFromRequestWithTrustedProxies(r, trustedProxies)
		},
		urlGuard: urlGuard,
	}
}

func (h *Handler) Close() error {
	closer, ok := h.statsStore.(statsStoreCloser)
	if !ok {
		return nil
	}
	return closer.Close()
}
