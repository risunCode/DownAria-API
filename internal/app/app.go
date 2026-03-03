package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"downaria-api/internal/core/config"
	"downaria-api/internal/shared/util"
	httptransport "downaria-api/internal/transport/http"
	"downaria-api/internal/transport/http/handlers"
	"downaria-api/internal/transport/http/middleware"
)

type Application struct {
	server       *http.Server
	shutdownHook func() error
}

func New(cfg config.Config) *Application {
	h := handlers.NewHandler(cfg, time.Now().UTC())
	router := httptransport.NewRouter(h, cfg)
	trustedProxies, err := util.NewIPAllowlist(cfg.TrustedProxyCIDRs)
	if err != nil {
		log.Printf("invalid TRUSTED_PROXY_CIDRS value; falling back to direct remote addr only: %v", err)
		trustedProxies = nil
	}

	limiter := middleware.NewRateLimiter(cfg.GlobalRateLimitLimit, cfg.GlobalRateLimitWindow)
	limiter.ConfigureBuckets(cfg.GlobalRateLimitMaxBuckets, cfg.GlobalRateLimitBucketTTL)
	limiter.SetClientIPLookup(func(r *http.Request) string {
		return util.ClientIPFromRequestWithTrustedProxies(r, trustedProxies)
	})
	handler := chain(
		router,
		middleware.CORS(cfg.AllowedOrigins),
		middleware.RequestID,
		middleware.StructuredLogging,
		middleware.RateLimit(limiter),
	)

	port := strings.TrimSpace(cfg.Port)
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           handler,
		ReadTimeout:       fallbackDuration(cfg.ServerReadTimeout, 15*time.Second),
		ReadHeaderTimeout: fallbackDuration(cfg.ServerReadHeaderTimeout, 10*time.Second),
		WriteTimeout:      fallbackDuration(cfg.ServerWriteTimeout, 30*time.Second),
		IdleTimeout:       fallbackDuration(cfg.ServerIdleTimeout, 60*time.Second),
		MaxHeaderBytes:    fallbackInt(cfg.ServerMaxHeaderBytes, 1<<20),
	}

	return &Application{server: server, shutdownHook: h.Close}
}

func (a *Application) Start() error {
	return a.server.ListenAndServe()
}

func (a *Application) Stop(ctx context.Context) error {
	var errs []error

	if err := a.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, err)
	}

	if a.shutdownHook != nil {
		if err := a.shutdownHook(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func chain(next http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	h := next
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func fallbackDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func fallbackInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
