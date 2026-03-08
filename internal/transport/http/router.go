package httptransport

import (
	"log"
	"net/http"

	"downaria-api/internal/core/config"
	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/transport/http/handlers"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/response"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(h *handlers.Handler, cfg config.Config) http.Handler {
	originProtected := middleware.RequireOrigin(cfg.AllowedOrigins)
	antiBot := middleware.BlockBotAccess()
	webSignature := middleware.RequireWebSignature(cfg.WebInternalSharedSecret)
	mergeEnabled := middleware.RequireMergeEnabled(cfg.MergeEnabled)
	r := chi.NewRouter()

	r.Use(chimiddleware.Recoverer)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		response.WriteErrorRequest(w, req, apperrors.HTTPStatus(apperrors.CodeNotFound), apperrors.CodeNotFound, "route not found, available prefixes are /api/v1/ (public) and /api/web/ (frontend)")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		response.WriteErrorRequest(w, req, apperrors.HTTPStatus(apperrors.CodeMethodNotAllowed), apperrors.CodeMethodNotAllowed, "method not allowed on this path")
	})

	r.Get("/", h.Root)
	r.Get("/health", h.Health)
	r.Get("/api/settings", h.Settings)
	r.Get("/api/v1/stats/public", h.PublicStats)
	r.Get("/metrics", h.Metrics)

	r.Route("/api/web", func(web chi.Router) {
		web.Use(originProtected)
		web.Use(webSignature)
		web.Use(antiBot)
		web.Post("/extract", h.Extract)
		web.Get("/proxy", h.Proxy)
		web.Get("/download", h.Download)
		web.With(mergeEnabled).Post("/merge", h.Merge)
	})

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Post("/extract", h.Extract)
		v1.Get("/proxy", h.Proxy)
		v1.Get("/download", h.Download)
		if cfg.WebInternalSharedSecret == "" {
			v1.With(mergeEnabled).Post("/merge", h.Merge)
		} else {
			log.Println("WARN: /api/v1/merge route is disabled because WEB_INTERNAL_SHARED_SECRET is configured")
		}
	})

	return r
}
