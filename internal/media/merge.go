package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	logging "downaria-api/internal/logging"
	"downaria-api/internal/netutil"
	"downaria-api/internal/platform/ytdlp"
	runtime "downaria-api/internal/runtime"
	"downaria-api/internal/storage"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MergeRequest struct {
	URL          string
	Quality      string
	VideoURL     string
	AudioURL     string
	Filename     string
	Format       string
	UserAgent    string
	CookieHeader string
}

type MergeResult struct {
	FilePath           string
	Filename           string
	ContentType        string
	ContentBytes       int64
	SelectionMode      string
	DownloadMethod     string
	SelectedFormatIDs  []string
	SelectedSourceURLs []string
}

type downloadedArtifact struct {
	Path   string
	Method string
}

type Merger struct {
	ffmpegPath     string
	ffprobePath    string
	downloader     *Downloader
	ytdlpPath      string
	metadata       extract.UniversalExtractor
	extractor      SourceExtractor
	logger         *slog.Logger
	maxOutputBytes int64
	maxDuration    time.Duration
	workspaceRoot  string
	workspaceTTL   time.Duration
}

type SourceExtractor interface {
	Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error)
}

type convertProbeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

type ffprobeResult struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width,omitempty"`
		Height    int    `json:"height,omitempty"`
	} `json:"streams"`
}

func NewMerger(dl *Downloader, ytdlpBinary string, sourceExtractor SourceExtractor, maxOutputBytes int64, maxDuration time.Duration, loggers ...*slog.Logger) *Merger {
	ffmpegPath := ResolveBinary("", "ffmpeg", "ffmpeg.exe")
	ffprobePath := ResolveBinary("", "ffprobe", "ffprobe.exe")
	ytdlpPath := ResolveBinary(strings.TrimSpace(ytdlpBinary), "yt-dlp", "yt-dlp.exe")
	var metadata extract.UniversalExtractor
	if ytdlpPath != "" {
		metadata = ytdlp.NewExtractor(ytdlpPath, nil)
	}
	workspaceRoot := runtime.Subdir("workspaces")
	_ = os.MkdirAll(workspaceRoot, 0o755)
	return &Merger{
		ffmpegPath:     ffmpegPath,
		ffprobePath:    ffprobePath,
		downloader:     dl,
		ytdlpPath:      ytdlpPath,
		metadata:       metadata,
		extractor:      sourceExtractor,
		logger:         logging.FallbackLogger(loggers...),
		maxOutputBytes: maxOutputBytes,
		maxDuration:    maxDuration,
		workspaceRoot:  workspaceRoot,
		workspaceTTL:   30 * time.Minute,
	}
}

func (s *Merger) SetWorkspaceTTL(value time.Duration) {
	if s != nil && value > 0 {
		s.workspaceTTL = value
	}
}

func (s *Merger) Merge(ctx context.Context, req MergeRequest, update func(storage.JobUpdate)) (*MergeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.URL) != "" {
		s.logInfo(ctx, "merge_stage", slog.String("stage", "source_request"), slog.String("url", req.URL), slog.String("quality", req.Quality), slog.String("format", req.Format))
		return s.mergeFromSource(ctx, req, update)
	}
	if strings.TrimSpace(req.VideoURL) == "" || strings.TrimSpace(req.AudioURL) == "" {
		return nil, extract.WrapCode(extract.KindInvalidInput, "merge_pair_required", "video_url and audio_url are required", false, nil)
	}
	if s.ffmpegPath == "" {
		return nil, extract.WrapCode(extract.KindInternal, "ffmpeg_unavailable", "ffmpeg is unavailable", false, nil)
	}
	if s.downloader == nil {
		return nil, extract.WrapCode(extract.KindInternal, "downloader_unavailable", "downloader is unavailable", false, nil)
	}
	format := normalizeMergeFormat(req.Format)
	filename := sanitizeMergeOutputName(req.Filename, format)
	videoResult, audioResult, err := s.downloader.DownloadPair(ctx,
		DownloadRequest{URL: req.VideoURL, Filename: "video-source", UserAgent: req.UserAgent, CookieHeader: netutil.CookieForSameHost(req.CookieHeader, req.VideoURL, req.AudioURL)},
		DownloadRequest{URL: req.AudioURL, Filename: "audio-source", UserAgent: req.UserAgent, CookieHeader: netutil.CookieForSameHost(req.CookieHeader, req.AudioURL, req.VideoURL)},
	)
	if err != nil {
		return nil, err
	}
	if update != nil {
		update(storage.JobUpdate{State: storage.StateMerging})
	}
	defer downloaderCleanup(videoResult)
	defer downloaderCleanup(audioResult)
	outputPath, info, err := s.mergeLocalFiles(ctx, videoResult.FilePath, audioResult.FilePath, format)
	if err != nil {
		return nil, err
	}
	return &MergeResult{FilePath: outputPath, Filename: filename, ContentType: mergeContentTypeFromFormat(format), ContentBytes: info.Size(), SelectionMode: "explicit_pair", SelectedSourceURLs: []string{req.VideoURL, req.AudioURL}}, nil
}

func (s *Merger) mergeFromSource(ctx context.Context, req MergeRequest, update func(storage.JobUpdate)) (*MergeResult, error) {
	if s.extractor == nil && (s.ytdlpPath == "" || s.metadata == nil) {
		return nil, extract.WrapCode(extract.KindInternal, "ytdlp_unavailable", "yt-dlp is unavailable", false, nil)
	}
	format := normalizeMergeFormat(req.Format)
	filename := sanitizeMergeOutputName(req.Filename, format)
	result, err := s.extractFromSource(ctx, req.URL, req.CookieHeader)
	if err != nil {
		return nil, err
	}
	result, err = extract.FinalizeResult(result, req.URL, result.Platform, result.ExtractProfile)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Filename) == "" {
		filename = mergeFilenameFromExtract(result, format)
	}
	if err := s.validateDuration(result); err != nil {
		return nil, err
	}
	excludedURLs := map[string]struct{}{}
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		selection, err := selectWithExcludedURLs(result, req.Quality, format, excludedURLs)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

		if selection.Mode == "progressive" {
			if err := s.preflightCandidate(ctx, selection.Video, req); err != nil {
				lastErr = err
				s.logWarn(ctx, "merge_stage", slog.String("stage", "candidate_preflight_failed"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode), slog.String("video_url", strings.TrimSpace(selection.Video.URL)), slog.String("error", extract.SafeMessage(err)))
				if markSelectionSourcesExcluded(selection, excludedURLs) {
					s.logInfo(ctx, "merge_stage", slog.String("stage", "candidate_fallback_switch"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode))
					continue
				}
				return nil, err
			}
		}

		tmpDir, err := s.workspaceDir(req, selection)
		if err != nil {
			return nil, extract.WrapCode(extract.KindInternal, "merge_workspace_create_failed", "failed to create temp dir", false, err)
		}

		if selection.Mode == "progressive" {
			artifact, err := s.downloadCandidate(ctx, req.URL, filepath.Join(tmpDir, "progressive"), selection.Video, req, update)
			if err != nil {
				lastErr = err
				s.logWarn(ctx, "merge_stage", slog.String("stage", "candidate_download_failed"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode), slog.String("video_url", strings.TrimSpace(selection.Video.URL)), slog.String("error", extract.SafeMessage(err)))
				_ = os.RemoveAll(tmpDir)
				if markSelectionSourcesExcluded(selection, excludedURLs) {
					s.logInfo(ctx, "merge_stage", slog.String("stage", "candidate_fallback_switch"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode))
					continue
				}
				return nil, err
			}
			if err := s.verifyMediaArtifact(ctx, artifact.Path, true, true); err != nil {
				_ = os.RemoveAll(tmpDir)
				return nil, err
			}
			if err := s.validateRequestedQuality(ctx, artifact.Path, req.Quality); err != nil {
				_ = os.RemoveAll(tmpDir)
				return nil, err
			}
			outputPath := artifact.Path
			if shouldConvertProgressiveForFormat(format, selection.Video, artifact.Path) {
				if s.ffmpegPath == "" {
					_ = os.RemoveAll(tmpDir)
					return nil, extract.WrapCode(extract.KindInternal, "ffmpeg_unavailable", "ffmpeg is unavailable", false, nil)
				}
				outputPath = filepath.Join(tmpDir, filename)
				if _, err := s.runFFmpegContainerConvert(ctx, artifact.Path, format, outputPath); err != nil {
					_ = os.RemoveAll(tmpDir)
					return nil, err
				}
				if err := s.verifyMediaArtifact(ctx, outputPath, true, true); err != nil {
					_ = os.RemoveAll(tmpDir)
					return nil, err
				}
				if err := s.validateRequestedQuality(ctx, outputPath, req.Quality); err != nil {
					_ = os.RemoveAll(tmpDir)
					return nil, err
				}
				if outputPath != artifact.Path {
					_ = os.Remove(artifact.Path)
				}
			}
			info, err := os.Stat(outputPath)
			if err != nil {
				_ = os.RemoveAll(tmpDir)
				return nil, extract.WrapCode(extract.KindInternal, "artifact_missing", "downloaded artifact not found", false, err)
			}
			if err := s.ensureOutputSize(info.Size()); err != nil {
				_ = os.RemoveAll(tmpDir)
				return nil, err
			}
			return &MergeResult{FilePath: outputPath, Filename: filename, ContentType: mergeContentTypeFromFormat(format), ContentBytes: info.Size(), SelectionMode: selection.Mode, DownloadMethod: artifact.Method, SelectedFormatIDs: selection.SelectedIDs, SelectedSourceURLs: collectSelectionURLs(selection)}, nil
		}

		if s.ffmpegPath == "" {
			_ = os.RemoveAll(tmpDir)
			return nil, extract.WrapCode(extract.KindInternal, "ffmpeg_unavailable", "ffmpeg is unavailable", false, nil)
		}
		videoArtifact, audioArtifact, err := s.downloadSelectionPair(ctx, req, tmpDir, selection, update)
		if err != nil {
			lastErr = err
			s.logWarn(ctx, "merge_stage", slog.String("stage", "candidate_pair_download_failed"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode), slog.String("video_url", strings.TrimSpace(selection.Video.URL)), slog.String("audio_url", strings.TrimSpace(selection.Audio.URL)), slog.String("error", extract.SafeMessage(err)))
			_ = os.RemoveAll(tmpDir)
			if markSelectionSourcesExcluded(selection, excludedURLs) {
				s.logInfo(ctx, "merge_stage", slog.String("stage", "candidate_fallback_switch"), slog.Int("attempt", attempt+1), slog.String("mode", selection.Mode))
				continue
			}
			return nil, err
		}
		outputPath := filepath.Join(tmpDir, filename)
		if _, err := s.runFFmpegMerge(ctx, videoArtifact.Path, audioArtifact.Path, format, outputPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, err
		}
		if err := s.verifyMediaArtifact(ctx, outputPath, true, true); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, err
		}
		if err := s.validateRequestedQuality(ctx, outputPath, req.Quality); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, err
		}
		info, err := os.Stat(outputPath)
		if err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, extract.WrapCode(extract.KindInternal, "merge_output_missing", "merged output not found", false, err)
		}
		if err := s.ensureOutputSize(info.Size()); err != nil {
			_ = os.RemoveAll(tmpDir)
			return nil, err
		}
		return &MergeResult{FilePath: outputPath, Filename: filename, ContentType: mergeContentTypeFromFormat(format), ContentBytes: info.Size(), SelectionMode: selection.Mode, DownloadMethod: joinDownloadMethods(videoArtifact.Method, audioArtifact.Method), SelectedFormatIDs: selection.SelectedIDs, SelectedSourceURLs: collectSelectionURLs(selection)}, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, extract.WrapCode(extract.KindExtractionFailed, "selector_no_match", "no source matched request", false, nil)
}

func (s *Merger) mergeLocalFiles(ctx context.Context, videoPath, audioPath, format string) (string, os.FileInfo, error) {
	mergeRoot, err := runtime.EnsureSubdir("merges")
	if err != nil {
		return "", nil, extract.WrapCode(extract.KindInternal, "merge_output_create_failed", "failed to create output file", false, err)
	}
	tmpDir, err := os.MkdirTemp(mergeRoot, "merge-*")
	if err != nil {
		return "", nil, extract.WrapCode(extract.KindInternal, "merge_output_create_failed", "failed to create output directory", false, err)
	}
	outputPath := filepath.Join(tmpDir, "output."+format)
	if _, err := s.runFFmpegMerge(ctx, videoPath, audioPath, format, outputPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	if err := s.verifyMediaArtifact(ctx, outputPath, true, true); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, extract.WrapCode(extract.KindInternal, "merge_output_missing", "merged output not found", false, err)
	}
	if err := s.ensureOutputSize(info.Size()); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	return outputPath, info, nil
}
func downloaderCleanup(result *DownloadResult) {
	if result == nil || result.FilePath == "" {
		return
	}
	_ = cleanupResult(result)
}

func collectSelectionURLs(selection *Selection) []string {
	urls := make([]string, 0, 2)
	if selection != nil && selection.Video != nil && strings.TrimSpace(selection.Video.URL) != "" {
		urls = append(urls, selection.Video.URL)
	}
	if selection != nil && selection.Audio != nil && strings.TrimSpace(selection.Audio.URL) != "" {
		urls = append(urls, selection.Audio.URL)
	}
	return urls
}

func (s *Merger) workspaceDir(req MergeRequest, selection *Selection) (string, error) {
	root := s.workspaceRoot
	if strings.TrimSpace(root) == "" {
		root = runtime.Subdir("workspaces")
	}
	dir := filepath.Join(root, workspaceKey(req, selection))
	if info, err := os.Stat(dir); err == nil && s.workspaceTTL > 0 && time.Since(info.ModTime()) > s.workspaceTTL {
		_ = os.RemoveAll(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func workspaceKey(req MergeRequest, selection *Selection) string {
	selected := ""
	if selection != nil {
		selected = strings.Join(selection.SelectedIDs, ",")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(req.URL) + "\n" + strings.TrimSpace(req.VideoURL) + "\n" + strings.TrimSpace(req.AudioURL) + "\n" + strings.TrimSpace(req.Quality) + "\n" + strings.TrimSpace(req.Format) + "\n" + shortCookieHash(req.CookieHeader) + "\n" + selected))
	return hex.EncodeToString(sum[:8])
}

func existingOutputPath(outputBase string) string {
	matches, err := filepath.Glob(outputBase + ".*")
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func shortCookieHash(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(cookie))
	return hex.EncodeToString(sum[:])[:8]
}

func (s *Merger) ensureOutputSize(size int64) error {
	if s == nil || s.maxOutputBytes <= 0 || size <= s.maxOutputBytes {
		return nil
	}
	return extract.WrapCode(extract.KindMergeFailed, "merge_output_too_large", "merged output exceeds configured size limit", false, nil)
}

func (s *Merger) validateDuration(result *extract.Result) error {
	if s == nil || s.maxDuration <= 0 || result == nil {
		return nil
	}
	limit := s.maxDuration.Seconds()
	for _, item := range result.Media {
		for _, source := range item.Sources {
			if source.DurationSeconds > limit {
				return extract.WrapCode(extract.KindInvalidInput, "media_duration_exceeded", "media duration exceeds configured limit", false, nil)
			}
		}
	}
	return nil
}
func (s *Merger) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if s == nil || s.logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("request_id", middleware.FromContext(ctx))}, attrs...)
	s.logger.LogAttrs(ctx, level, message, attrs...)
}

func (s *Merger) logInfo(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelInfo, msg, attrs...)
}

func (s *Merger) logWarn(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelWarn, msg, attrs...)
}

func normalizeMergeFormat(value string) string {
	format := strings.ToLower(strings.TrimSpace(value))
	if format == "" {
		return "mp4"
	}
	return format
}

func sanitizeMergeOutputName(name string, format string) string {
	base := extract.SanitizeFilename(name, "merged")
	return extract.EnsureFilenameExtension(base, format)
}

func mergeFilenameFromExtract(result *extract.Result, format string) string {
	if result == nil {
		return sanitizeMergeOutputName("", format)
	}
	if strings.TrimSpace(result.Filename) != "" {
		return extract.EnsureFilenameExtension(strings.TrimSpace(result.Filename), format)
	}
	return sanitizeMergeOutputName("", format)
}

func shouldConvertProgressiveForFormat(format string, candidate *Candidate, artifactPath string) bool {
	format = normalizeMergeFormat(format)
	if format != "mp4" {
		return false
	}
	if normalizeContainer(strings.TrimPrefix(strings.TrimSpace(filepath.Ext(artifactPath)), ".")) == "mp4" {
		return false
	}
	if candidate == nil {
		return true
	}
	return normalizeContainer(candidate.Container) != "mp4"
}

func markSelectionSourcesExcluded(selection *Selection, excluded map[string]struct{}) bool {
	if selection == nil || excluded == nil {
		return false
	}
	marked := false
	if selection.Video != nil && strings.TrimSpace(selection.Video.URL) != "" {
		excluded[strings.TrimSpace(selection.Video.URL)] = struct{}{}
		marked = true
	}
	if selection.Audio != nil && strings.TrimSpace(selection.Audio.URL) != "" {
		excluded[strings.TrimSpace(selection.Audio.URL)] = struct{}{}
		marked = true
	}
	return marked
}

func mergeContentTypeFromFormat(format string) string {
	if value := mime.TypeByExtension("." + format); value != "" {
		return value
	}
	if format == "mkv" {
		return "video/x-matroska"
	}
	return "video/mp4"
}
