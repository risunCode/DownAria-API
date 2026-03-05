package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/response"
)

// HLSStream handles HLS playlist and segment streaming with automatic URL rewriting
func (h *Handler) HLSStream(w http.ResponseWriter, r *http.Request) {
	builder := response.NewBuilderFromRequest(r).WithAccessMode("public").WithPublicContent(true)

	target := r.URL.Query().Get("url")
	if target == "" {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "query parameter 'url' is required")
		return
	}

	requestID := middleware.RequestIDFromContext(r.Context())
	validatedTarget, err := h.sanitizeAndValidateOutboundURL(r.Context(), target)
	if err != nil {
		log.Printf("request_id=%s component=hls_stream event=url_validation_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "url is required and must point to a public http/https destination")
		return
	}
	target = validatedTarget

	upstreamAuth := resolveUpstreamAuthorization(r)
	userAgentOverride := strings.TrimSpace(r.Header.Get("X-Upstream-User-Agent"))
	headers := buildProxyHeaders(target, userAgentOverride, upstreamAuth, requestID)

	// Determine if this is a playlist or segment
	isPlaylist := strings.HasSuffix(strings.ToLower(target), ".m3u8")

	result, err := h.Streamer.Stream(r.Context(), network.StreamOptions{
		URL:     target,
		Headers: headers,
	})

	if err != nil {
		log.Printf("request_id=%s component=hls_stream event=stream_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to stream HLS content")
		return
	}
	defer result.Body.Close()

	// If this is a playlist, rewrite URLs
	if isPlaylist {
		maxBytes := int64(10 * 1024 * 1024) // 10MB max for playlists
		playlistContent, err := io.ReadAll(io.LimitReader(result.Body, maxBytes))
		if err != nil {
			log.Printf("request_id=%s component=hls_stream event=playlist_read_failed err=%s", requestID, err)
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to read HLS playlist")
			return
		}

		// Determine proxy base URL
		proxyBaseURL := h.config.PublicBaseURL
		if proxyBaseURL == "" {
			proxyBaseURL = fmt.Sprintf("http://localhost:%s", h.config.Port)
		}

		// Rewrite playlist URLs to point to /api/v1/hls-stream (public, no signature)
		rewrittenContent, err := rewriteHLSPlaylistForRoute(playlistContent, target, proxyBaseURL, "/api/v1/hls-stream")
		if err != nil {
			log.Printf("request_id=%s component=hls_stream event=playlist_rewrite_failed err=%s", requestID, err)
			// Fallback to original content if rewrite fails
			rewrittenContent = playlistContent
		}

		// Write rewritten playlist
		w.Header().Set("Content-Type", result.ContentType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rewrittenContent)))
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(rewrittenContent)
		return
	}

	// For segments (.ts, .m4s, etc), just proxy through
	w.Header().Set("Content-Type", result.ContentType)
	if cl := strings.TrimSpace(result.Headers["Content-Length"]); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400") // Segments are immutable
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(result.StatusCode)

	maxBytes := int64(100 * 1024 * 1024) // 100MB max for segments
	_, _ = io.Copy(w, io.LimitReader(result.Body, maxBytes))

	if result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusBadRequest {
		h.statsStore.RecordDownload(time.Now().UTC())
	}
}
