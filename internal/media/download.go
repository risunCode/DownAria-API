package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/melbahja/got"

	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	logging "downaria-api/internal/logging"
	"downaria-api/internal/outbound"
	runtime "downaria-api/internal/runtime"
	"downaria-api/internal/storage"
)

type DownloadRequest struct {
	URL          string
	Filename     string
	Platform     string
	UserAgent    string
	CookieHeader string
	Referer      string
	Origin       string
	TempRoot     string
}

type DownloadResult struct {
	FilePath     string
	Filename     string
	ContentType  string
	ContentBytes int64
	Method       string
}

type PreflightResult struct {
	ContentType string
	FinalURL    string
}

type Downloader struct {
	client    *outbound.Client
	ytdlpPath string
	logger    *slog.Logger
	maxBytes  int64
}

func NewDownloader(client *outbound.Client, maxBytes int64, ytdlpBinary string, loggers ...*slog.Logger) *Downloader {
	ytdlpPath := ResolveBinary(strings.TrimSpace(ytdlpBinary), "yt-dlp", "yt-dlp.exe")
	return &Downloader{client: client, ytdlpPath: ytdlpPath, logger: logging.FallbackLogger(loggers...), maxBytes: maxBytes}
}

func (s *Downloader) Download(ctx context.Context, req DownloadRequest, update func(storage.JobUpdate)) (*DownloadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	targetURL := strings.TrimSpace(req.URL)
	if targetURL == "" {
		return nil, extract.WrapCode(extract.KindInvalidInput, "download_url_required", "url is required", false, nil)
	}
	filename := sanitizeFilename(req.Filename, targetURL)

	return s.downloadWithGot(ctx, req, filename, update)
}

func (s *Downloader) Preflight(ctx context.Context, req DownloadRequest) (*PreflightResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.client == nil {
		return nil, extract.WrapCode(extract.KindInternal, "downloader_unavailable", "downloader is unavailable", false, nil)
	}
	headers := buildHeaders(req)
	headers["Range"] = "bytes=0-0"
	headers["Accept"] = "*/*"
	resp, err := s.client.Get(ctx, req.URL, headers)
	if err != nil {
		return nil, extract.WrapCode(extract.KindUpstreamFailure, "download_preflight_failed", "download preflight failed", true, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, extract.WrapCode(extract.KindUpstreamFailure, "download_preflight_status", fmt.Sprintf("upstream returned status %d", resp.StatusCode), resp.StatusCode >= http.StatusInternalServerError, nil)
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/json") {
		return nil, extract.WrapCode(extract.KindUpstreamFailure, "download_preflight_invalid_content", "upstream did not return media content", true, nil)
	}
	finalURL := req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if err := s.client.ValidateURL(ctx, finalURL); err != nil {
		return nil, extract.WrapCode(extract.KindUpstreamFailure, "download_preflight_redirect_invalid", "redirected media URL failed validation", false, err)
	}
	return &PreflightResult{ContentType: contentType, FinalURL: finalURL}, nil
}

func (s *Downloader) DownloadPair(ctx context.Context, videoReq DownloadRequest, audioReq DownloadRequest) (*DownloadResult, *DownloadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type pairResult struct {
		kind   string
		result *DownloadResult
		err    error
	}
	results := make(chan pairResult, 2)
	var wg sync.WaitGroup

	start := func(kind string, req DownloadRequest) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := s.Download(ctx, req, nil)
			results <- pairResult{kind: kind, result: result, err: err}
		}()
	}

	start("video", videoReq)
	start("audio", audioReq)

	go func() {
		wg.Wait()
		close(results)
	}()

	var videoResult *DownloadResult
	var audioResult *DownloadResult
	for result := range results {
		if result.err != nil {
			cancel()
			if videoResult != nil {
				_ = cleanupResult(videoResult)
			}
			if audioResult != nil {
				_ = cleanupResult(audioResult)
			}
			return nil, nil, result.err
		}
		if result.kind == "video" {
			videoResult = result.result
		} else {
			audioResult = result.result
		}
	}
	if videoResult == nil || audioResult == nil {
		if videoResult != nil {
			_ = cleanupResult(videoResult)
		}
		if audioResult != nil {
			_ = cleanupResult(audioResult)
		}
		return nil, nil, extract.WrapCode(extract.KindDownloadFailed, "download_pair_incomplete", "failed to download media pair", true, nil)
	}
	return videoResult, audioResult, nil
}

func (s *Downloader) downloadWithGot(ctx context.Context, req DownloadRequest, filename string, update func(storage.JobUpdate)) (*DownloadResult, error) {
	if err := s.client.ValidateURL(ctx, req.URL); err != nil {
		return nil, extract.WrapCode(extract.KindUpstreamFailure, "download_request_failed", "download request failed", false, err)
	}

	downloadRoot, err := ensureDownloadRoot(req.TempRoot)
	if err != nil {
		return nil, extract.WrapCode(extract.KindInternal, "download_temp_create_failed", "failed to create temp directory", false, err)
	}
	tmpDir, err := os.MkdirTemp(downloadRoot, "got-*")
	if err != nil {
		return nil, extract.WrapCode(extract.KindInternal, "download_temp_create_failed", "failed to create temp directory", false, err)
	}
	targetPath := filepath.Join(tmpDir, filepath.Base(filename))

	// Configure Got downloader
	g := got.New()
	g.Client = s.client.HTTPClientWithoutTimeout()

	headers := buildHeaders(req)
	gotHeaders := make([]got.GotHeader, 0, len(headers))
	for k, v := range headers {
		gotHeaders = append(gotHeaders, got.GotHeader{Key: k, Value: v})
	}

	download := &got.Download{
		URL:         req.URL,
		Dest:        targetPath,
		Header:      gotHeaders,
		Concurrency: 8, // Support multi-connection for speed
		Interval:    500,
	}

	s.logInfo(ctx, "download_stage", slog.String("stage", "got_start"), slog.String("url", req.URL))

	// Handle progress
	stopProgress := make(chan struct{})
	if update != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					pct := 0.0
					if total := download.TotalSize(); total > 0 {
						pct = float64(download.Size()) / float64(total) * 100
					}
					update(storage.JobUpdate{
						Progress: int(pct),
						Message:  fmt.Sprintf("Downloading to server: %.1f%% (%.2f MB/s) [%s / %s]", pct, float64(download.Speed())/1024/1024, fmtSize(download.Size()), fmtSize(download.TotalSize())),
					})
				case <-stopProgress:
					return
				}
			}
		}()
	}

	err = g.Do(download)
	close(stopProgress)

	if err != nil {
		_ = os.RemoveAll(tmpDir)
		s.logWarn(ctx, "download_stage", slog.String("stage", "got_failed"), slog.String("url", req.URL), slog.String("error", err.Error()))
		return nil, extract.WrapCode(extract.KindDownloadFailed, "download_save_failed", fmt.Sprintf("failed to save download: %v", err), true, err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, extract.WrapCode(extract.KindInternal, "download_temp_create_failed", "failed to stat download file", false, err)
	}
	if s.maxBytes > 0 && info.Size() > s.maxBytes {
		_ = os.RemoveAll(tmpDir)
		return nil, extract.WrapCode(extract.KindDownloadFailed, "download_too_large", "download exceeds configured size limit", false, nil)
	}

	// Move file out of isolated temp dir to a stable location
	finalRoot, _ := runtime.EnsureSubdir("final")
	finalPath := filepath.Join(finalRoot, filepath.Base(targetPath))
	if err := atomicMove(targetPath, finalPath); err == nil {
		_ = os.RemoveAll(tmpDir)
		return &DownloadResult{FilePath: finalPath, Filename: filename, ContentType: detectContentType(finalPath, filename), ContentBytes: info.Size(), Method: "got"}, nil
	}

	return &DownloadResult{FilePath: targetPath, Filename: filename, ContentType: detectContentType(targetPath, filename), ContentBytes: info.Size(), Method: "got"}, nil
}

func fmtSize(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func atomicMove(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fallback to copy+delete if partitions are different
	return moveByCopy(src, dst)
}

func moveByCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Remove(src)
}

func buildHeaders(req DownloadRequest) map[string]string {
	headers := map[string]string{}
	if ua := strings.TrimSpace(req.UserAgent); ua != "" {
		headers[sanitizeHeaderToken("User-Agent")] = sanitizeHeaderValue(ua)
	}
	if cookie := strings.TrimSpace(req.CookieHeader); cookie != "" {
		headers[sanitizeHeaderToken("Cookie")] = sanitizeHeaderValue(cookie)
	}
	if referer := strings.TrimSpace(req.Referer); referer != "" {
		headers[sanitizeHeaderToken("Referer")] = sanitizeHeaderValue(referer)
	}
	if origin := strings.TrimSpace(req.Origin); origin != "" {
		headers[sanitizeHeaderToken("Origin")] = sanitizeHeaderValue(origin)
	}
	return headers
}

func sanitizeFilename(value string, rawURL string) string {
	return extract.SanitizeFilename(value, filepath.Base(strings.TrimSpace(rawURL)))
}

func ensureDownloadRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return runtime.EnsureSubdir("downloads")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}

func contentTypeFromName(name string) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); value != "" {
		return value
	}
	return "application/octet-stream"
}

func cleanupResult(result *DownloadResult) error {
	if result == nil || result.FilePath == "" {
		return nil
	}
	return os.RemoveAll(filepath.Dir(result.FilePath))
}

func detectContentType(path string, fallbackName string) string {
	if value := contentTypeFromName(path); value != "application/octet-stream" {
		return value
	}
	if value := contentTypeFromName(fallbackName); value != "application/octet-stream" {
		return value
	}
	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	if n <= 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func (s *Downloader) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if s == nil || s.logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("request_id", middleware.FromContext(ctx))}, attrs...)
	s.logger.LogAttrs(ctx, level, message, attrs...)
}

func (s *Downloader) logInfo(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelInfo, msg, attrs...)
}

func (s *Downloader) logWarn(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelWarn, msg, attrs...)
}
