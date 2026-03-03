package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	apperrors "fetchmoona/internal/core/errors"
	"fetchmoona/internal/infra/network"
	"fetchmoona/internal/shared/util"
	"fetchmoona/internal/transport/http/middleware"
	"fetchmoona/pkg/response"
)

const proxyHeadCacheTTL = 45 * time.Second

const (
	maxProxyPreviewSizeMB = 10 * 1024 // 10 GB
	defaultDownloadSizeMB = 1024      // 1 GB
)

type proxyHeadMetadata struct {
	StatusCode    int
	ContentType   string
	ContentLength string
}

func (h *Handler) Proxy(w http.ResponseWriter, r *http.Request) {
	h.proxyWithMode(w, r, false)
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	h.proxyWithMode(w, r, true)
}

func (h *Handler) proxyWithMode(w http.ResponseWriter, r *http.Request, forceDownload bool) {
	builder := response.NewBuilderFromRequest(r).WithAccessMode("public").WithPublicContent(true)

	target := r.URL.Query().Get("url")
	if target == "" {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "query parameter 'url' is required")
		return
	}

	requestID := middleware.RequestIDFromContext(r.Context())
	validatedTarget, err := h.sanitizeAndValidateOutboundURL(r.Context(), target)
	if err != nil {
		log.Printf("request_id=%s component=proxy event=url_validation_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "url is required and must point to a public http/https destination")
		return
	}
	target = validatedTarget

	upstreamAuth := resolveUpstreamAuthorization(r)
	userAgentOverride := strings.TrimSpace(r.Header.Get("X-Upstream-User-Agent"))

	headers := buildProxyHeaders(target, userAgentOverride, upstreamAuth, requestID)
	rangeHeader := r.Header.Get("Range")
	headOnly := r.URL.Query().Get("head") == "1"
	isDownload := forceDownload || r.URL.Query().Get("download") == "1"

	if headOnly {
		cacheKey := buildProxyHeadCacheKey(target, upstreamAuth, userAgentOverride)
		meta, err := h.getProxyHeadMetadata(r.Context(), cacheKey, target, headers)
		if err != nil {
			log.Printf("request_id=%s component=proxy event=head_request_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to fetch upstream headers")
			return
		}
		if meta.StatusCode >= http.StatusBadRequest {
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, fmt.Sprintf("upstream returned status %d", meta.StatusCode))
			return
		}
		writeProxyHeadResponse(w, meta)
		return
	}

	result, err := h.Streamer.Stream(r.Context(), network.StreamOptions{
		URL:         target,
		Headers:     headers,
		RangeHeader: rangeHeader,
	})

	if err != nil {
		log.Printf("request_id=%s component=proxy event=stream_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeProxyFailed), apperrors.CodeProxyFailed, "failed to stream upstream media")
		return
	}
	defer result.Body.Close()

	if cl := result.Headers["Content-Length"]; cl != "" {
		if size, err := strconv.ParseInt(cl, 10, 64); err == nil {
			maxBytes := h.proxySizeLimitBytes(isDownload)
			if size > maxBytes {
				limitMB := h.proxySizeLimitMB(isDownload)
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge,
					fmt.Sprintf("file size %d MB exceeds maximum %d MB", size/(1024*1024), limitMB))
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
	maxBytes := h.proxySizeLimitBytes(isDownload)
	_, _ = io.Copy(w, io.LimitReader(result.Body, maxBytes))

	if isDownload && result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusBadRequest {
		h.statsStore.RecordDownload(time.Now().UTC())
	}
}

func (h *Handler) proxySizeLimitMB(isDownload bool) int {
	if !isDownload {
		return maxProxyPreviewSizeMB
	}
	if h.config.MaxDownloadSizeMB > 0 {
		return h.config.MaxDownloadSizeMB
	}
	return defaultDownloadSizeMB
}

func (h *Handler) proxySizeLimitBytes(isDownload bool) int64 {
	return int64(h.proxySizeLimitMB(isDownload)) * 1024 * 1024
}

func (h *Handler) getProxyHeadMetadata(ctx context.Context, cacheKey, targetURL string, headers map[string]string) (proxyHeadMetadata, error) {
	if h.headCache != nil {
		if cached, ok := h.headCache.Get(cacheKey); ok {
			if meta, castOK := cached.(proxyHeadMetadata); castOK {
				return meta, nil
			}
		}
	}

	v, err, _ := h.headGroup.Do(cacheKey, func() (any, error) {
		if h.headCache != nil {
			if cached, ok := h.headCache.Get(cacheKey); ok {
				if meta, castOK := cached.(proxyHeadMetadata); castOK {
					return meta, nil
				}
			}
		}

		meta, fetchErr := h.fetchProxyHeadMetadata(ctx, targetURL, headers)
		if fetchErr != nil {
			return proxyHeadMetadata{}, fetchErr
		}
		if h.headCache != nil {
			ttl := h.config.CacheProxyHeadTTL
			if ttl <= 0 {
				ttl = proxyHeadCacheTTL
			}
			h.headCache.Set(cacheKey, meta, ttl)
		}
		return meta, nil
	})
	if err != nil {
		return proxyHeadMetadata{}, err
	}
	meta, _ := v.(proxyHeadMetadata)
	return meta, nil
}

func (h *Handler) fetchProxyHeadMetadata(ctx context.Context, targetURL string, headers map[string]string) (proxyHeadMetadata, error) {
	meta := proxyHeadMetadata{}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		return meta, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()

	meta.StatusCode = resp.StatusCode
	meta.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	meta.ContentLength = strings.TrimSpace(resp.Header.Get("Content-Length"))
	return meta, nil
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

	if referer, origin := deriveUpstreamOriginAndReferer(targetURL); referer != "" {
		headers["Referer"] = referer
		if origin != "" {
			headers["Origin"] = origin
		}
	}

	return headers
}

func deriveUpstreamOriginAndReferer(targetURL string) (referer string, origin string) {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return "", ""
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.TrimSpace(parsed.Host)
	if host == "" || (scheme != "http" && scheme != "https") {
		return "", ""
	}

	origin = scheme + "://" + host
	return origin + "/", origin
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

	return strings.TrimSpace(r.Header.Get("X-Upstream-Authorization"))
}

func buildProxyHeadCacheKey(targetURL string, authorization string, userAgent string) string {
	authHash := sha256.Sum256([]byte(strings.TrimSpace(authorization)))
	uaHash := sha256.Sum256([]byte(strings.TrimSpace(userAgent)))
	return fmt.Sprintf("url=%s|auth_sha256=%s|ua_sha256=%s", strings.TrimSpace(targetURL), hex.EncodeToString(authHash[:]), hex.EncodeToString(uaHash[:]))
}
