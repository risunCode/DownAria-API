package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	apperrors "downaria-api/internal/core/errors"
	"downaria-api/internal/infra/network"
	"downaria-api/internal/shared/security"
	"downaria-api/internal/shared/util"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/response"
)

const proxyHeadCacheTTL = 45 * time.Second

type proxyHeadMetadata struct {
	StatusCode    int
	ContentType   string
	ContentLength string
}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	builder := response.NewBuilderFromRequest(r).WithAccessMode("public").WithPublicContent(true)

	target := r.URL.Query().Get("url")
	if target == "" {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "query parameter 'url' is required")
		return
	}

	requestID := middleware.RequestIDFromContext(r.Context())
	guard := h.urlGuard
	if guard == nil {
		guard = security.NewOutboundURLValidator(nil)
	}
	validatedTarget, err := guard.Validate(r.Context(), target)
	if err != nil {
		log.Printf("request_id=%s component=proxy event=url_validation_failed err=%v", requestID, err)
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "url is required and must point to a public http/https destination")
		return
	}
	target = validatedTarget.String()

	upstreamAuth := resolveUpstreamAuthorization(r)
	userAgentOverride := strings.TrimSpace(r.Header.Get("X-Upstream-User-Agent"))

	headers := buildProxyHeaders(target, userAgentOverride, upstreamAuth, requestID)
	rangeHeader := r.Header.Get("Range")
	headOnly := r.URL.Query().Get("head") == "1"
	isDownload := r.URL.Query().Get("download") == "1"

	if headOnly {
		cacheKey := buildProxyHeadCacheKey(target, upstreamAuth, userAgentOverride)
		if cached, ok := h.headCache.Get(cacheKey); ok {
			if meta, castOK := cached.(proxyHeadMetadata); castOK {
				writeProxyHeadResponse(w, meta)
				return
			}
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodHead, target, nil)
		if err != nil {
			log.Printf("request_id=%s component=proxy event=create_head_request_failed err=%v", requestID, err)
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, apperrors.Message(apperrors.CodeProxyFailed))
			return
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := h.httpClient.Do(req)
		if err != nil {
			log.Printf("request_id=%s component=proxy event=head_request_failed err=%v", requestID, err)
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to fetch upstream headers")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, fmt.Sprintf("upstream returned status %d", resp.StatusCode))
			return
		}

		meta := proxyHeadMetadata{
			StatusCode:    resp.StatusCode,
			ContentType:   strings.TrimSpace(resp.Header.Get("Content-Type")),
			ContentLength: strings.TrimSpace(resp.Header.Get("Content-Length")),
		}
		h.headCache.Set(cacheKey, meta, proxyHeadCacheTTL)
		writeProxyHeadResponse(w, meta)
		return
	}

	result, err := h.Streamer.Stream(r.Context(), network.StreamOptions{
		URL:         target,
		Headers:     headers,
		RangeHeader: rangeHeader,
	})

	if err != nil {
		log.Printf("request_id=%s component=proxy event=stream_failed err=%v", requestID, err)
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to stream upstream media")
		return
	}
	defer result.Body.Close()

	if cl := result.Headers["Content-Length"]; cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			maxBytes := int64(h.config.MaxDownloadSizeMB) * 1024 * 1024
			if size > maxBytes {
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge,
					fmt.Sprintf("file size %d MB exceeds maximum %d MB", size/(1024*1024), h.config.MaxDownloadSizeMB))
				return
			}
		}
	}

	w.Header().Set("Content-Type", result.ContentType)
	if cl := strings.TrimSpace(result.Headers["Content-Length"]); cl != "" {
		w.Header().Set("Content-Length", cl)
		w.Header().Set("X-File-Size", cl)
	}
	if etag := strings.TrimSpace(result.Headers["Etag"]); etag != "" {
		w.Header().Set("ETag", etag)
	}
	if lastModified := strings.TrimSpace(result.Headers["Last-Modified"]); lastModified != "" {
		w.Header().Set("Last-Modified", lastModified)
	}
	if strings.HasPrefix(strings.ToLower(result.ContentType), "image/") {
		w.Header().Set("Cache-Control", "public, max-age=604800, stale-while-revalidate=86400")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	if cr := result.Headers["Content-Range"]; cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	w.Header().Set("Accept-Ranges", "bytes")

	if isDownload {
		requestedFilename := strings.TrimSpace(r.URL.Query().Get("filename"))
		platform := strings.TrimSpace(r.URL.Query().Get("platform"))
		finalFilename := resolveDownloadFilename(requestedFilename, result.Headers["Content-Disposition"], target, platform, result.ContentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", finalFilename))
	} else if upstreamDisposition := strings.TrimSpace(result.Headers["Content-Disposition"]); upstreamDisposition != "" {
		w.Header().Set("Content-Disposition", upstreamDisposition)
	}

	w.WriteHeader(result.StatusCode)
	maxBytes := int64(h.config.MaxDownloadSizeMB) * 1024 * 1024
	_, _ = io.Copy(w, io.LimitReader(result.Body, maxBytes))

	if isDownload && result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusBadRequest {
		h.statsStore.RecordDownload(time.Now().UTC())
	}
}

// buildProxyHeaders builds platform-specific headers for proxy requests
func buildProxyHeaders(targetURL, userAgent, authorization, requestID string) map[string]string {
	headers := make(map[string]string)
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = util.DefaultUserAgent
	}
	headers["User-Agent"] = ua

	if auth := strings.TrimSpace(authorization); auth != "" {
		headers["Authorization"] = auth
	}

	if reqID := strings.TrimSpace(requestID); reqID != "" {
		headers["X-Request-ID"] = reqID
	}

	lowerURL := strings.ToLower(targetURL)
	switch {
	case strings.Contains(lowerURL, "instagram.com") || strings.Contains(lowerURL, "cdninstagram.com") || strings.Contains(lowerURL, "instagram.") || strings.Contains(lowerURL, "threads.com") || strings.Contains(lowerURL, "threads.net"):
		headers["Referer"] = "https://www.instagram.com/"
	case strings.Contains(lowerURL, "facebook.com") || strings.Contains(lowerURL, "fbcdn.net"):
		headers["Referer"] = "https://www.facebook.com/"
	case strings.Contains(lowerURL, "googlevideo.com") || strings.Contains(lowerURL, "youtube.com"):
		headers["Referer"] = "https://www.youtube.com/"
		headers["Origin"] = "https://www.youtube.com"
	case strings.Contains(lowerURL, "pixiv.net") || strings.Contains(lowerURL, "pximg.net"):
		headers["Referer"] = "https://www.pixiv.net/"
	case strings.Contains(lowerURL, "twitter.com") || strings.Contains(lowerURL, "x.com") || strings.Contains(lowerURL, "twimg.com"):
		headers["Referer"] = "https://x.com/"
		headers["Origin"] = "https://x.com"
	case strings.Contains(lowerURL, "tiktok.com") || strings.Contains(lowerURL, "tiktokcdn.com") || strings.Contains(lowerURL, "byteoversea.com"):
		headers["Referer"] = "https://www.tiktok.com/"
		headers["Origin"] = "https://www.tiktok.com"
	}

	return headers
}

func writeProxyHeadResponse(w http.ResponseWriter, meta proxyHeadMetadata) {
	if meta.ContentLength != "" {
		w.Header().Set("X-File-Size", meta.ContentLength)
	}
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusNoContent)
}

func resolveUpstreamAuthorization(r *http.Request) string {
	if r == nil {
		return ""
	}

	if value := strings.TrimSpace(r.Header.Get("X-Upstream-Authorization")); value != "" {
		return value
	}

	return strings.TrimSpace(r.URL.Query().Get("upstream_auth"))
}

func buildProxyHeadCacheKey(targetURL string, authorization string, userAgent string) string {
	authHash := sha256.Sum256([]byte(strings.TrimSpace(authorization)))
	uaHash := sha256.Sum256([]byte(strings.TrimSpace(userAgent)))
	return fmt.Sprintf("url=%s|auth_sha256=%s|ua_sha256=%s", strings.TrimSpace(targetURL), hex.EncodeToString(authHash[:]), hex.EncodeToString(uaHash[:]))
}
