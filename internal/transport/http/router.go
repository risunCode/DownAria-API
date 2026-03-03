package httptransport

import (
	"net/http"

	"downaria-api/internal/core/config"
	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/transport/http/handlers"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/response"
	"github.com/go-chi/chi/v5"
)

func NewRouter(h *handlers.Handler, cfg config.Config) http.Handler {
	originProtected := middleware.RequireOrigin(cfg.AllowedOrigins)
	antiBot := middleware.BlockBotAccess()
	webSignature := middleware.RequireWebSignature(cfg.WebInternalSharedSecret)
	r := chi.NewRouter()

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		response.WriteErrorRequest(w, req, apperrors.HTTPStatus(apperrors.CodeNotFound), apperrors.CodeNotFound, "route not found, available prefixes are /api/v1/ (public) and /api/web/ (frontend)")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		response.WriteErrorRequest(w, req, apperrors.HTTPStatus(apperrors.CodeMethodNotAllowed), apperrors.CodeMethodNotAllowed, "method not allowed on this path")
	})

	r.Get("/health", h.Health)
	r.Get("/api/settings", h.Settings)
	r.Get("/api/v1/stats/public", h.PublicStats)

	r.Route("/api/web", func(web chi.Router) {
		web.Use(originProtected)
		web.Use(antiBot)
		web.Use(webSignature)
		web.Post("/extract", h.Extract)
		web.Get("/proxy", h.Proxy)
		web.Post("/merge", h.Merge)
	})

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Post("/extract", h.Extract)
		v1.Get("/proxy", h.Proxy)
		v1.Post("/merge", h.Merge)
	})

	return r
}
