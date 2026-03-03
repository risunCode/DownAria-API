package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	extractcore "fetchmoona/internal/extractors/core"
	"fetchmoona/internal/infra/network"
)

var contentRangeTotalRe = regexp.MustCompile(`(?i)^bytes\s+\d+-\d+\/(\d+|\*)$`)

func (h *Handler) enrichVariantSizes(ctx context.Context, result *extractcore.ExtractResult, requestID string) {
	if result == nil || !shouldEnrichPlatform(result.Platform) {
		return
	}

	type variantRef struct {
		mediaIndex   int
		variantIndex int
	}

	byURL := make(map[string][]variantRef)
	for mi := range result.Media {
		for vi := range result.Media[mi].Variants {
			variant := &result.Media[mi].Variants[vi]
			if variant == nil || variant.Size > 0 {
				continue
			}
			targetURL := strings.TrimSpace(variant.URL)
			if targetURL == "" {
				continue
			}
			byURL[targetURL] = append(byURL[targetURL], variantRef{mediaIndex: mi, variantIndex: vi})
		}
	}

	if len(byURL) == 0 {
		return
	}

	const maxConcurrent = 6
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	sizeByURL := make(map[string]int64, len(byURL))

	for targetURL := range byURL {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			size := h.resolveRemoteContentLength(ctx, url, requestID)
			if size <= 0 {
				return
			}

			mu.Lock()
			sizeByURL[url] = size
			mu.Unlock()
		}(targetURL)
	}

	wg.Wait()

	for targetURL, refs := range byURL {
		size := sizeByURL[targetURL]
		if size <= 0 {
			continue
		}
		for _, ref := range refs {
			result.Media[ref.mediaIndex].Variants[ref.variantIndex].Size = size
			result.Media[ref.mediaIndex].Variants[ref.variantIndex].Filesize = size
		}
	}
}

func shouldEnrichPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "facebook", "instagram", "twitter", "x", "pixiv", "tiktok":
		return true
	case "threads":
		return true
	case "youtube", "generic":
		return true
	default:
		return false
	}
}

func (h *Handler) resolveRemoteContentLength(ctx context.Context, targetURL, requestID string) int64 {
	validatedURL, err := h.sanitizeAndValidateOutboundURL(ctx, targetURL)
	if err != nil {
		return 0
	}
	targetURL = validatedURL

	cacheKey := buildProxyHeadCacheKey(targetURL, "", "")
	if h.headCache != nil {
		if cached, ok := h.headCache.Get(cacheKey); ok {
			if meta, castOK := cached.(proxyHeadMetadata); castOK {
				if size := parseContentLength(meta.ContentLength); size > 0 {
					return size
				}
			}
		}
	}

	headers := buildProxyHeaders(targetURL, "", "", requestID)
	meta, _ := h.getProxyHeadMetadata(ctx, cacheKey, targetURL, headers)
	if size := parseContentLength(meta.ContentLength); size > 0 {
		h.setHeadMetadataCache(cacheKey, meta)
		return size
	}

	rangeMeta := h.fetchRangeMetadata(ctx, targetURL, headers)
	if rangeMeta.ContentLength != "" {
		meta.ContentLength = rangeMeta.ContentLength
	}
	if rangeMeta.ContentType != "" {
		meta.ContentType = rangeMeta.ContentType
	}
	if rangeMeta.StatusCode > 0 {
		meta.StatusCode = rangeMeta.StatusCode
	}

	if size := parseContentLength(meta.ContentLength); size > 0 {
		h.setHeadMetadataCache(cacheKey, meta)
		return size
	}

	if size := h.probeStreamContentLength(ctx, targetURL, headers); size > 0 {
		meta.ContentLength = strconv.FormatInt(size, 10)
		h.setHeadMetadataCache(cacheKey, meta)
		return size
	}

	return parseContentLength(meta.ContentLength)
}

func (h *Handler) setHeadMetadataCache(cacheKey string, meta proxyHeadMetadata) {
	if parseContentLength(meta.ContentLength) <= 0 {
		return
	}

	ttl := h.config.CacheProxyHeadTTL
	if ttl <= 0 {
		ttl = proxyHeadCacheTTL
	}
	if h.headCache != nil {
		h.headCache.Set(cacheKey, meta, ttl)
	}
}

func (h *Handler) probeStreamContentLength(ctx context.Context, targetURL string, headers map[string]string) int64 {
	streamResult, err := h.Streamer.Stream(ctx, network.StreamOptions{
		URL:         targetURL,
		Headers:     headers,
		RangeHeader: "bytes=0-65535",
	})
	if err != nil || streamResult == nil || streamResult.Body == nil {
		return 0
	}
	defer streamResult.Body.Close()

	if size := parseContentLength(streamResult.Headers["Content-Length"]); size > 0 {
		return size
	}

	if contentRange := strings.TrimSpace(streamResult.Headers["Content-Range"]); contentRange != "" {
		if total := parseContentRangeTotal(contentRange); total > 0 {
			return total
		}
	}

	return 0
}

func (h *Handler) fetchHeadMetadata(ctx context.Context, targetURL string, headers map[string]string) proxyHeadMetadata {
	meta := proxyHeadMetadata{}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		return meta
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return meta
	}
	defer resp.Body.Close()

	meta.StatusCode = resp.StatusCode
	meta.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	meta.ContentLength = strings.TrimSpace(resp.Header.Get("Content-Length"))
	return meta
}

func (h *Handler) fetchRangeMetadata(ctx context.Context, targetURL string, headers map[string]string) proxyHeadMetadata {
	meta := proxyHeadMetadata{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return meta
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return meta
	}
	defer resp.Body.Close()

	meta.StatusCode = resp.StatusCode
	meta.ContentType = strings.TrimSpace(resp.Header.Get("Content-Type"))

	if contentRange := strings.TrimSpace(resp.Header.Get("Content-Range")); contentRange != "" {
		if total := parseContentRangeTotal(contentRange); total > 0 {
			meta.ContentLength = strconv.FormatInt(total, 10)
			return meta
		}
	}

	meta.ContentLength = strings.TrimSpace(resp.Header.Get("Content-Length"))
	return meta
}

func parseContentRangeTotal(value string) int64 {
	match := contentRangeTotalRe.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 || match[1] == "*" {
		return 0
	}
	total, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || total <= 0 {
		return 0
	}
	return total
}

func parseContentLength(value string) int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	size, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || size <= 0 {
		return 0
	}
	return size
}
