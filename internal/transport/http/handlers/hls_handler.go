package handlers

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	apperrors "downaria-api/internal/core/errors"
	infrahls "downaria-api/internal/infra/hls"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/response"
	"github.com/grafov/m3u8"
)

func (h *Handler) HandleHLSRequest(w http.ResponseWriter, r *http.Request, routePath string) {
	builder := response.NewBuilderFromRequest(r).WithAccessMode("public").WithPublicContent(true)
	target := strings.TrimSpace(r.URL.Query().Get("url"))
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

	chunk := parseChunkParam(r.URL.Query())
	headers := buildProxyHeaders(target, strings.TrimSpace(r.Header.Get("X-Upstream-User-Agent")), resolveUpstreamAuthorization(r), requestID)

	h.metrics.IncActiveHLS()
	defer h.metrics.DecActiveHLS()
	start := time.Now()

	if chunk {
		result, err := h.Streamer.Stream(r.Context(), network.StreamOptions{URL: target, Headers: headers})
		if err != nil {
			h.metrics.AddFailure()
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeHLSSegmentFetchFailed), apperrors.CodeHLSSegmentFetchFailed, "failed to fetch hls segment")
			return
		}
		defer result.Body.Close()
		w.Header().Set("Content-Type", result.ContentType)
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(result.StatusCode)
		n, err := h.streamingDownloader.StreamWithBuffer(r.Context(), result.Body, w, result.ContentType)
		if err == nil {
			h.metrics.AddDownload(n, time.Since(start))
		}
		return
	}

	result, err := h.Streamer.Stream(r.Context(), network.StreamOptions{URL: target, Headers: headers})
	if err != nil {
		h.metrics.AddFailure()
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to stream HLS content")
		return
	}
	defer result.Body.Close()

	body, err := io.ReadAll(io.LimitReader(result.Body, 10*1024*1024))
	if err != nil {
		h.metrics.AddFailure()
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeHLSPlaylistParseFailed), apperrors.CodeHLSPlaylistParseFailed, "failed to read HLS playlist")
		return
	}

	rewritten := body
	if isHLSPlaylist(target, result.ContentType) {
		playlist, listType, parseErr := h.hlsParser.ParsePlaylist(body)
		if parseErr == nil {
			switch listType {
			case m3u8.MASTER:
				master := playlist.(*m3u8.MasterPlaylist)
				infrahls.RewriteMasterPlaylist(master, target, routePath)
				rewritten = master.Encode().Bytes()
			case m3u8.MEDIA:
				media := playlist.(*m3u8.MediaPlaylist)
				infrahls.RewriteMediaPlaylist(media, target, routePath)
				rewritten = media.Encode().Bytes()
			}
		}
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(rewritten)
	h.metrics.AddDownload(int64(len(rewritten)), time.Since(start))
}
