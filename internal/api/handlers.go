package api

import (
	"context"
	"downaria-api/internal/extract"
	"downaria-api/internal/media"
	"downaria-api/internal/netutil"
	runtime "downaria-api/internal/runtime"
	"downaria-api/internal/storage"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleSmartDownload(ctx context.Context, req MediaRequest, extractSvc ExtractService, downloadSvc DownloadService, convertSvc ConvertService, mergeSvc MergeService, update func(storage.JobUpdate)) (*smartDownloadResult, error) {
	if shouldUseExtractedAudioPath(req, extractSvc) {
		return handleExtractedAudioDownload(ctx, req, extractSvc, downloadSvc, convertSvc, update)
	}
	if shouldUseConvertPath(req) {
		if convertSvc == nil {
			return nil, extract.WrapCode(extract.KindInternal, "convert_service_unavailable", "convert service is not configured", false, nil)
		}
		result, err := convertSvc.Convert(ctx, media.ConvertRequest{
			URL:          req.URL,
			Filename:     req.Filename,
			Format:       req.Format,
			AudioOnly:    req.AudioOnly,
			UserAgent:    extract.FirstNonEmpty(req.UserAgent, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
			CookieHeader: netutil.CookieForSameHost(req.Auth.Cookie, req.URL, req.URL),
		})
		if err != nil {
			return nil, err
		}
		return &smartDownloadResult{FilePath: result.FilePath, Filename: result.Filename, ContentType: result.ContentType, ContentBytes: result.ContentBytes, SelectionMode: "convert", DownloadMethod: "ffmpeg"}, nil
	}
	if shouldUseMergePath(req, extractSvc) {
		if mergeSvc == nil {
			return nil, extract.WrapCode(extract.KindInternal, "merge_service_unavailable", "merge service is not configured", false, nil)
		}
		result, err := mergeSvc.Merge(ctx, media.MergeRequest{URL: req.URL, Quality: req.Quality, VideoURL: req.VideoURL, AudioURL: req.AudioURL, Filename: req.Filename, Format: req.Format, UserAgent: req.UserAgent, CookieHeader: mergeCookieHeader(req)}, update)
		if err != nil {
			return nil, err
		}
		return &smartDownloadResult{FilePath: result.FilePath, Filename: result.Filename, ContentType: result.ContentType, ContentBytes: result.ContentBytes, SelectionMode: extract.FirstNonEmpty(strings.TrimSpace(result.SelectionMode), "merge"), DownloadMethod: extract.FirstNonEmpty(strings.TrimSpace(result.DownloadMethod), "merge")}, nil
	}
	if downloadSvc == nil {
		return nil, extract.WrapCode(extract.KindInternal, "download_service_unavailable", "download service is not configured", false, nil)
	}
	result, err := downloadSvc.Download(ctx, media.DownloadRequest{
		URL:          req.URL,
		Filename:     req.Filename,
		Platform:     req.Platform,
		UserAgent:    extract.FirstNonEmpty(req.UserAgent, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		CookieHeader: netutil.CookieForSameHost(req.Auth.Cookie, req.URL, req.URL),
	}, update)
	if err != nil {
		return nil, err
	}
	return &smartDownloadResult{FilePath: result.FilePath, Filename: result.Filename, ContentType: result.ContentType, ContentBytes: result.ContentBytes, SelectionMode: "direct", DownloadMethod: extract.FirstNonEmpty(strings.TrimSpace(result.Method), "direct")}, nil
}

func handleExtractedAudioDownload(ctx context.Context, req MediaRequest, extractSvc ExtractService, downloadSvc DownloadService, convertSvc ConvertService, update func(storage.JobUpdate)) (*smartDownloadResult, error) {
	if extractSvc == nil {
		return nil, extract.WrapCode(extract.KindInternal, "extract_service_unavailable", "extract service is not configured", false, nil)
	}
	if downloadSvc == nil {
		return nil, extract.WrapCode(extract.KindInternal, "download_service_unavailable", "download service is not configured", false, nil)
	}
	result, err := extractSvc.Extract(ctx, req.URL, extract.ExtractOptions{CookieHeader: req.Auth.Cookie, UseAuth: req.Auth.Cookie != ""})
	if err != nil {
		return nil, err
	}
	source := selectBestAudioSource(result, req.Format)
	if source == nil {
		return nil, extract.WrapCode(extract.KindExtractionFailed, "selector_no_match", "no audio source matched request", false, nil)
	}
	filename := strings.TrimSpace(req.Filename)
	if filename == "" && result != nil {
		filename = result.Filename
	}
	if filename != "" {
		ext := strings.TrimSpace(req.Format)
		if ext == "" {
			ext = strings.TrimSpace(source.Container)
		}
		filename = extract.EnsureFilenameExtension(filename, ext)
	}
	if isHLSLikeAudioSource(source) && convertSvc == nil {
		return nil, extract.WrapCode(extract.KindInternal, "convert_service_unavailable", "convert service is not configured", false, nil)
	}
	if canDirectDownloadAudioSource(strings.TrimSpace(req.Format), source) || convertSvc == nil {
		downloaded, err := downloadSvc.Download(ctx, media.DownloadRequest{
			URL:          source.URL,
			Filename:     filename,
			Platform:     result.Platform,
			UserAgent:    extract.FirstNonEmpty(req.UserAgent, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
			CookieHeader: netutil.CookieForSameHost(req.Auth.Cookie, req.URL, source.URL),
			Referer:      source.Referer,
			Origin:       source.Origin,
		}, update)
		if err != nil {
			return nil, err
		}
		return &smartDownloadResult{FilePath: downloaded.FilePath, Filename: downloaded.Filename, ContentType: downloaded.ContentType, ContentBytes: downloaded.ContentBytes, SelectionMode: "audio_direct", DownloadMethod: extract.FirstNonEmpty(strings.TrimSpace(downloaded.Method), "direct")}, nil
	}
	converted, err := convertSvc.Convert(ctx, media.ConvertRequest{
		URL:          source.URL,
		Filename:     filename,
		Format:       req.Format,
		AudioOnly:    true,
		UserAgent:    extract.FirstNonEmpty(req.UserAgent, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		CookieHeader: netutil.CookieForSameHost(req.Auth.Cookie, req.URL, source.URL),
	})
	if err != nil {
		return nil, err
	}
	return &smartDownloadResult{FilePath: converted.FilePath, Filename: converted.Filename, ContentType: converted.ContentType, ContentBytes: converted.ContentBytes, SelectionMode: "audio_convert", DownloadMethod: "ffmpeg"}, nil
}

func shouldUseConvertPath(req MediaRequest) bool {
	if req.AudioOnly {
		return true
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	return format == "mp3"
}

func shouldUseExtractedAudioPath(req MediaRequest, extractSvc ExtractService) bool {
	if !req.AudioOnly || extractSvc == nil {
		return false
	}
	return !looksLikeDirectMediaURL(req.URL)
}

func shouldUseMergePath(req MediaRequest, extractSvc ExtractService) bool {
	if strings.TrimSpace(req.VideoURL) != "" || strings.TrimSpace(req.AudioURL) != "" {
		return true
	}
	if strings.TrimSpace(req.Quality) != "" {
		return true
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format != "" && format != "mp4" {
		return true
	}
	if extractSvc == nil {
		return false
	}
	return !looksLikeDirectMediaURL(req.URL)
}

func looksLikeDirectMediaURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	mime := strings.ToLower(strings.TrimSpace(parsed.Query().Get("mime")))
	if strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/") {
		return true
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(parsed.Path)))
	switch ext {
	case ".mp4", ".m4a", ".mp3", ".webm", ".mov", ".mkv", ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func selectBestAudioSource(result *extract.Result, desiredFormat string) *extract.MediaSource {
	if result == nil {
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(desiredFormat))
	var best *extract.MediaSource
	bestScore := -1
	for _, item := range result.Media {
		if strings.TrimSpace(item.Type) != "audio" {
			continue
		}
		for i := range item.Sources {
			source := item.Sources[i]
			score := audioSourceScore(source, format)
			if score > bestScore {
				copy := source
				best = &copy
				bestScore = score
			}
		}
	}
	return best
}

func audioSourceScore(source extract.MediaSource, desiredFormat string) int {
	score := 0
	container := strings.ToLower(strings.TrimSpace(source.Container))
	protocol := strings.ToLower(strings.TrimSpace(source.Protocol))
	if desiredFormat != "" && container == desiredFormat {
		score += 100
	}
	if protocol == "http" || protocol == "https" {
		score += 50
	}
	if source.FileSizeBytes > 0 {
		score += 10
	}
	if container == "mp3" {
		score += 5
	}
	if protocol == "m3u8_native" {
		score -= 10
	}
	return score
}

func canDirectDownloadAudioSource(format string, source *extract.MediaSource) bool {
	if source == nil {
		return false
	}
	if isHLSLikeAudioSource(source) {
		return false
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return true
	}
	return strings.ToLower(strings.TrimSpace(source.Container)) == format
}

func isHLSLikeAudioSource(source *extract.MediaSource) bool {
	if source == nil {
		return false
	}
	protocol := strings.ToLower(strings.TrimSpace(source.Protocol))
	if protocol == "m3u8" || protocol == "m3u8_native" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(source.URL)), ".m3u8")
}

func statusFromError(err error) int {
	switch {
	case extract.IsKind(err, extract.KindInvalidInput):
		return http.StatusBadRequest
	case extract.IsKind(err, extract.KindAuthRequired):
		return http.StatusUnauthorized
	case extract.IsKind(err, extract.KindUnsupportedPlatform):
		return http.StatusUnprocessableEntity
	case extract.IsKind(err, extract.KindUpstreamFailure):
		return http.StatusBadGateway
	case extract.IsKind(err, extract.KindRateLimited):
		return http.StatusTooManyRequests
	case extract.IsKind(err, extract.KindTimeout):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func mergeCookieHeader(req MediaRequest) string {
	cookie := strings.TrimSpace(req.Auth.Cookie)
	if cookie == "" {
		return ""
	}

	videoURL := strings.TrimSpace(req.VideoURL)
	audioURL := strings.TrimSpace(req.AudioURL)
	if videoURL != "" && audioURL != "" {
		if netutil.SameHost(videoURL, audioURL) {
			return cookie
		}
		return ""
	}
	if videoURL != "" {
		return netutil.CookieForSameHost(cookie, videoURL, videoURL)
	}
	if audioURL != "" {
		return netutil.CookieForSameHost(cookie, audioURL, audioURL)
	}
	if sourceURL := strings.TrimSpace(req.URL); sourceURL != "" {
		return netutil.CookieForSameHost(cookie, sourceURL, sourceURL)
	}
	return ""
}

func cleanupDownloadOutput(path string) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return
	}

	root := filepath.Clean(runtime.Root())
	outputDir := filepath.Clean(filepath.Dir(cleanPath))
	rel, err := filepath.Rel(root, outputDir)
	if err == nil && rel != "" && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		_ = os.RemoveAll(outputDir)
		return
	}

	_ = os.Remove(cleanPath)
}

func serveFileDownload(w http.ResponseWriter, r *http.Request, path, filename, contentType string, size int64) {
	// Disable write deadline for long file transfers.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	file, err := os.Open(path)
	if err != nil {
		writeError(w, r, extract.WrapCode(extract.KindInternal, "artifact_open_failed", "failed to open output file", false, err))
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, r, extract.WrapCode(extract.KindInternal, "artifact_stat_failed", "failed to inspect output file", false, err))
		return
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", extract.AttachmentDisposition(filename))
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func streamMedia(w http.ResponseWriter, r *http.Request, options RouterOptions, targetURL, filename, userAgent, cookieHeader string) {
	if options.OutboundClient == nil {
		writeError(w, r, extract.WrapCode(extract.KindInternal, "proxy_client_unavailable", "outbound client is not configured", false, nil))
		return
	}

	// Disable write deadline for long streaming
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		writeError(w, r, extract.WrapCode(extract.KindInternal, "proxy_request_build_failed", "failed to build upstream request", false, err))
		return
	}

	// Forward important headers
	copyHeader(req.Header, r.Header, "Range")
	copyHeader(req.Header, r.Header, "Accept")

	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}

	// Execute request
	resp, err := proxyHTTPClient(options.OutboundClient.HTTPClientWithoutTimeout()).Do(req)
	if err != nil {
		writeError(w, r, extract.WrapCode(extract.KindUpstreamFailure, "proxy_request_failed", "failed to fetch upstream media", true, err))
		return
	}
	defer resp.Body.Close()

	// Forward response headers
	copyHeader(w.Header(), resp.Header, "Content-Type")
	copyHeader(w.Header(), resp.Header, "Content-Length")
	copyHeader(w.Header(), resp.Header, "Content-Range")
	copyHeader(w.Header(), resp.Header, "Accept-Ranges")
	copyHeader(w.Header(), resp.Header, "ETag")
	copyHeader(w.Header(), resp.Header, "Last-Modified")
	copyHeader(w.Header(), resp.Header, "Cache-Control")

	// Override Content-Disposition to force download with correct filename
	if filename != "" {
		w.Header().Set("Content-Disposition", extract.AttachmentDisposition(filename))
	}

	if resp.StatusCode >= 400 {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 64*1024))
		return
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

const proxyResponseHeaderTimeout = 20 * time.Second

func handleProxy(options RouterOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetURL := strings.TrimSpace(r.URL.Query().Get("url"))
		if targetURL == "" {
			writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "proxy_url_required", "url parameter is required", false, nil))
			return
		}

		// Disable write deadline for long streaming
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Time{})

		if options.Security.Guard != nil {
			if _, err := options.Security.Guard.Validate(r.Context(), targetURL); err != nil {
				writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "proxy_url_blocked", "requested url is blocked", false, err))
				return
			}
		}

		if options.OutboundClient == nil {
			writeError(w, r, extract.WrapCode(extract.KindInternal, "proxy_client_unavailable", "outbound client is not configured", false, nil))
			return
		}

		// Build upstream request
		// We use a context without timeout for streaming if needed, but the client might have its own.
		// However, for media streaming, we typically want to respect the client's connection.
		ctx := r.Context()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			writeError(w, r, extract.WrapCode(extract.KindInternal, "proxy_request_build_failed", "failed to build upstream request", false, err))
			return
		}

		// Forward important headers
		copyHeader(req.Header, r.Header, "Range")
		copyHeader(req.Header, r.Header, "Accept")

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		// Execute request
		// We use the HTTPClient directly to have full control over the response
		resp, err := proxyHTTPClient(options.OutboundClient.HTTPClientWithoutTimeout()).Do(req)
		if err != nil {
			writeError(w, r, extract.WrapCode(extract.KindUpstreamFailure, "proxy_request_failed", "failed to fetch upstream media", true, err))
			return
		}
		defer resp.Body.Close()

		// Forward response headers
		copyHeader(w.Header(), resp.Header, "Content-Type")
		copyHeader(w.Header(), resp.Header, "Content-Length")
		copyHeader(w.Header(), resp.Header, "Content-Range")
		copyHeader(w.Header(), resp.Header, "Accept-Ranges")
		copyHeader(w.Header(), resp.Header, "ETag")
		copyHeader(w.Header(), resp.Header, "Last-Modified")
		copyHeader(w.Header(), resp.Header, "Cache-Control")

		// If it's a redirect, we might want to handle it, but http.Client already does by default.
		// If we get a non-2xx/206 status, we should probably forward it or error out.
		if resp.StatusCode >= 400 {
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, io.LimitReader(resp.Body, 64*1024))
			return
		}

		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}

func proxyHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{}
	}
	cloned := *base
	if transport, ok := base.Transport.(*http.Transport); ok {
		transportClone := transport.Clone()
		if transportClone.ResponseHeaderTimeout <= 0 || transportClone.ResponseHeaderTimeout > proxyResponseHeaderTimeout {
			transportClone.ResponseHeaderTimeout = proxyResponseHeaderTimeout
		}
		cloned.Transport = transportClone
	}
	cloned.Timeout = 0
	return &cloned
}

func copyHeader(dst, src http.Header, key string) {
	if val := src.Get(key); val != "" {
		dst.Set(key, val)
	}
}
