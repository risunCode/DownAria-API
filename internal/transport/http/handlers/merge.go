package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	apperrors "downaria-api/internal/core/errors"
	extractorcore "downaria-api/internal/extractors/core"
	"downaria-api/internal/shared/util"
	"downaria-api/internal/transport/http/middleware"
	"downaria-api/pkg/ffmpeg"
	"downaria-api/pkg/response"
)

type mergeRequest struct {
	VideoURL  string `json:"videoUrl,omitempty"`
	AudioURL  string `json:"audioUrl,omitempty"`
	URL       string `json:"url,omitempty"`
	Quality   string `json:"quality,omitempty"`
	Format    string `json:"format,omitempty"`
	Filename  string `json:"filename,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

func (h *Handler) Merge(w http.ResponseWriter, r *http.Request) {
	builder := response.NewBuilderFromRequest(r).
		WithAccessMode("public").
		WithPublicContent(true)

	if !h.config.MergeEnabled {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "merge endpoint is disabled")
		return
	}

	if strings.TrimSpace(h.config.WebInternalSharedSecret) != "" && strings.HasPrefix(r.URL.Path, "/api/v1/") {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "merge endpoint is restricted to signed /api/web routes")
		return
	}

	defer r.Body.Close()

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var req mergeRequest
	if err := decoder.Decode(&req); err != nil {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidJSON), apperrors.CodeInvalidJSON, apperrors.Message(apperrors.CodeInvalidJSON))
		return
	}

	targetURL := strings.TrimSpace(req.URL)
	videoURL := strings.TrimSpace(req.VideoURL)
	audioURL := strings.TrimSpace(req.AudioURL)
	usesDirectPair := videoURL != "" || audioURL != ""

	if usesDirectPair {
		if videoURL == "" || audioURL == "" {
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMissingParams), apperrors.CodeMissingParams, "videoUrl and audioUrl are required together")
			return
		}
	}

	if targetURL == "" {
		if !usesDirectPair {
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMissingParams), apperrors.CodeMissingParams, "url is required")
			return
		}
	}
	audioOnly := isAudioOnlyRequest(req)

	if !usesDirectPair && !isYouTubeURL(targetURL) && !audioOnly {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "video merge fast-path only supports YouTube URLs")
		return
	}

	requestID := middleware.RequestIDFromContext(r.Context())
	if !usesDirectPair {
		validatedTargetURL, err := h.sanitizeAndValidateOutboundURL(r.Context(), targetURL)
		if err != nil {
			log.Printf("request_id=%s component=merge event=url_validation_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "url is required and must point to a public http/https destination")
			return
		}
		targetURL = validatedTargetURL
	}

	ff := ffmpeg.New()
	if ff == nil {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFFmpegUnavailable), apperrors.CodeFFmpegUnavailable, apperrors.Message(apperrors.CodeFFmpegUnavailable))
		return
	}

	if usesDirectPair {
		validatedVideoURL, validateVideoErr := h.sanitizeAndValidateOutboundURL(r.Context(), videoURL)
		if validateVideoErr != nil {
			log.Printf("request_id=%s component=merge event=direct_video_url_blocked err=%s", requestID, redactLogError(validateVideoErr))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "videoUrl must point to a public http/https destination")
			return
		}

		validatedAudioURL, validateAudioErr := h.sanitizeAndValidateOutboundURL(r.Context(), audioURL)
		if validateAudioErr != nil {
			log.Printf("request_id=%s component=merge event=direct_audio_url_blocked err=%s", requestID, redactLogError(validateAudioErr))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeInvalidURL), apperrors.CodeInvalidURL, "audioUrl must point to a public http/https destination")
			return
		}

		mergeHeaders := buildMergeHeaders(validatedVideoURL, requestID)
		ffmpegCtx, cancelFFmpeg := withCommandTimeout(r.Context(), 4*time.Minute)
		defer cancelFFmpeg()

		result, err := ff.StreamMerge(ffmpegCtx, ffmpeg.MergeOptions{
			VideoURL:  validatedVideoURL,
			AudioURL:  validatedAudioURL,
			UserAgent: resolveUserAgent(req.UserAgent),
			Headers:   mergeHeaders,
		})
		if err != nil {
			log.Printf("request_id=%s component=merge event=direct_merge_start_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to start merge process")
			return
		}
		defer result.Close()

		filename := ensureFileExtension(req.Filename, "mp4")
		maxOutputBytes := int64(h.config.MaxMergeOutputSizeMB) * 1024 * 1024
		if err := writeFFmpegResultAsAttachment(w, result, filename, "video/mp4", maxOutputBytes); err != nil {
			if errors.Is(err, errMergeOutputExceeded) {
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge, fmt.Sprintf("merged output exceeds maximum %d MB", h.config.MaxMergeOutputSizeMB))
				return
			}
			if errors.Is(err, errMergeResponseWriteFailed) {
				log.Printf("request_id=%s component=merge event=direct_merged_response_write_failed err=%s", requestID, redactLogError(err))
				return
			}
			log.Printf("request_id=%s component=merge event=direct_merged_output_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to finalize merged output")
			return
		}
		return
	}

	mergeHeaders := buildMergeHeaders(targetURL, requestID)

	if audioOnly {
		audioFormat, codec, contentType := resolveAudioOutput(req.Format, req.Quality)
		inputURL := targetURL

		if isYouTubeURL(targetURL) {
			selector := buildYTDLPFormatSelector(req.Quality, true)
			ytCtx, cancelYTDLP := withCommandTimeout(r.Context(), 35*time.Second)
			urls, err := extractorcore.RunYtDlpGetURLs(ytCtx, targetURL, selector)
			cancelYTDLP()
			if err != nil {
				log.Printf("request_id=%s component=merge event=ytdlp_audio_resolve_failed err=%s", requestID, redactLogError(err))
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to resolve media stream")
				return
			}
			if len(urls) == 0 {
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to resolve media stream")
				return
			}

			validatedInputURL, validateErr := h.sanitizeAndValidateOutboundURL(r.Context(), urls[0])
			if validateErr != nil {
				log.Printf("request_id=%s component=merge event=ytdlp_audio_url_blocked err=%s", requestID, redactLogError(validateErr))
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "resolved stream URL is not allowed")
				return
			}
			inputURL = validatedInputURL
		}

		ffmpegCtx, cancelFFmpeg := withCommandTimeout(r.Context(), 4*time.Minute)
		defer cancelFFmpeg()
		result, err := ff.StreamExtractAudio(ffmpegCtx, ffmpeg.AudioExtractOptions{
			InputURL:   inputURL,
			OutputExt:  audioFormat,
			AudioCodec: codec,
			UserAgent:  resolveUserAgent(req.UserAgent),
			Headers:    mergeHeaders,
		})
		if err != nil {
			log.Printf("request_id=%s component=merge event=audio_extract_start_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to process audio stream")
			return
		}
		defer result.Close()

		filename := ensureFileExtension(req.Filename, audioFormat)
		maxOutputBytes := int64(h.config.MaxMergeOutputSizeMB) * 1024 * 1024
		if err := writeFFmpegResultAsAttachment(w, result, filename, contentType, maxOutputBytes); err != nil {
			if errors.Is(err, errMergeOutputExceeded) {
				_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge, fmt.Sprintf("merged output exceeds maximum %d MB", h.config.MaxMergeOutputSizeMB))
				return
			}
			if errors.Is(err, errMergeResponseWriteFailed) {
				log.Printf("request_id=%s component=merge event=audio_response_write_failed err=%s", requestID, redactLogError(err))
				return
			}
			log.Printf("request_id=%s component=merge event=audio_output_failed err=%s", requestID, redactLogError(err))
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to finalize audio output")
			return
		}
		return
	}

	selector := buildYTDLPFormatSelector(req.Quality, false)
	ytCtx, cancelYTDLP := withCommandTimeout(r.Context(), 35*time.Second)
	urls, err := extractorcore.RunYtDlpGetURLs(ytCtx, targetURL, selector)
	cancelYTDLP()
	if err != nil {
		log.Printf("request_id=%s component=merge event=ytdlp_stream_resolve_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to resolve media streams")
		return
	}

	if len(urls) < 2 {
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "yt-dlp fast-path requires separate video and audio streams")
		return
	}

	validatedVideoURL, validateVideoErr := h.sanitizeAndValidateOutboundURL(r.Context(), urls[0])
	if validateVideoErr != nil {
		log.Printf("request_id=%s component=merge event=ytdlp_video_url_blocked err=%s", requestID, redactLogError(validateVideoErr))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "resolved video stream URL is not allowed")
		return
	}
	validatedAudioURL, validateAudioErr := h.sanitizeAndValidateOutboundURL(r.Context(), urls[1])
	if validateAudioErr != nil {
		log.Printf("request_id=%s component=merge event=ytdlp_audio_url_blocked err=%s", requestID, redactLogError(validateAudioErr))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "resolved audio stream URL is not allowed")
		return
	}

	ffmpegCtx, cancelFFmpeg := withCommandTimeout(r.Context(), 4*time.Minute)
	defer cancelFFmpeg()
	result, err := ff.StreamMerge(ffmpegCtx, ffmpeg.MergeOptions{
		VideoURL:  validatedVideoURL,
		AudioURL:  validatedAudioURL,
		UserAgent: resolveUserAgent(req.UserAgent),
		Headers:   mergeHeaders,
	})
	if err != nil {
		log.Printf("request_id=%s component=merge event=merge_start_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to start merge process")
		return
	}
	defer result.Close()

	filename := ensureFileExtension(req.Filename, "mp4")
	maxOutputBytes := int64(h.config.MaxMergeOutputSizeMB) * 1024 * 1024
	if err := writeFFmpegResultAsAttachment(w, result, filename, "video/mp4", maxOutputBytes); err != nil {
		if errors.Is(err, errMergeOutputExceeded) {
			_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge, fmt.Sprintf("merged output exceeds maximum %d MB", h.config.MaxMergeOutputSizeMB))
			return
		}
		if errors.Is(err, errMergeResponseWriteFailed) {
			log.Printf("request_id=%s component=merge event=merged_response_write_failed err=%s", requestID, redactLogError(err))
			return
		}
		log.Printf("request_id=%s component=merge event=merged_output_failed err=%s", requestID, redactLogError(err))
		_ = builder.WriteError(w, apperrors.HTTPStatus(apperrors.CodeMergeFailed), apperrors.CodeMergeFailed, "failed to finalize merged output")
		return
	}
}

func withCommandTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= timeout {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

var errMergeOutputExceeded = errors.New("merge output exceeds configured limit")
var errMergeResponseWriteFailed = errors.New("failed to write merge response")

func writeFFmpegResultAsAttachment(w http.ResponseWriter, result *ffmpeg.FFmpegResult, filename, contentType string, maxOutputBytes int64) error {
	if result == nil || result.Stdout == nil {
		return fmt.Errorf("missing ffmpeg output stream")
	}
	if maxOutputBytes <= 0 {
		return fmt.Errorf("invalid max output size")
	}

	tmpFile, err := os.CreateTemp("", "downaria-merge-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	written, err := io.Copy(tmpFile, io.LimitReader(result.Stdout, maxOutputBytes+1))
	if err != nil {
		_ = tmpFile.Close()
		return err
	}

	if written > maxOutputBytes {
		_ = tmpFile.Close()
		_ = result.Close()
		return errMergeOutputExceeded
	}

	if err := result.Wait(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		_ = tmpFile.Close()
		return err
	}

	info, err := tmpFile.Stat()
	if err != nil {
		_ = tmpFile.Close()
		return err
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	if _, err := io.Copy(w, tmpFile); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("%w: %v", errMergeResponseWriteFailed, err)
	}

	return tmpFile.Close()
}

func resolveUserAgent(ua string) string {
	if strings.TrimSpace(ua) != "" {
		return strings.TrimSpace(ua)
	}
	return util.DefaultUserAgent
}

func buildMergeHeaders(targetURL, requestID string) map[string]string {
	headers := make(map[string]string)

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

func ensureFileExtension(filename, format string) string {
	filename = strings.TrimSpace(filename)
	filename = sanitizeUnsafeFilename(filename)
	filename = normalizeDownAriaTag(filename)
	if filename == "" {
		return "downaria_output." + format
	}

	ext := filepath.Ext(filename)
	if ext != "" {
		normalizedCandidate := strings.ToLower(strings.TrimPrefix(ext, "."))
		if !filenameExtAllowedRe.MatchString(normalizedCandidate) {
			ext = ""
		}
	}
	if ext != "" {
		normalizedExt := strings.ToLower(strings.TrimPrefix(ext, "."))
		normalizedFormat := strings.ToLower(strings.TrimSpace(format))
		base := strings.TrimSpace(strings.TrimSuffix(filename, ext))
		base = stripDownloadLabelSuffix(base)
		if base == "" {
			base = "downaria_output"
		}

		if normalizedExt == normalizedFormat {
			return base + "." + normalizedExt
		}
		if normalizedFormat == "" {
			return base + "." + normalizedExt
		}
		return base + "." + normalizedFormat
	}

	filename = stripDownloadLabelSuffix(filename)
	filename = normalizeDownAriaTag(filename)
	if filename == "" {
		filename = "downaria_output"
	}

	if strings.TrimSpace(format) == "" {
		return filename
	}

	return filename + "." + format
}

var filenameUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9 _\.\-\[\]\(\)]`)
var filenameUrlNoiseRe = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
var filenameSpaceRe = regexp.MustCompile(`\s+`)
var filenameUnderscoreRe = regexp.MustCompile(`_+`)
var downAriaTagRe = regexp.MustCompile(`(?i)\[downaria\]?`)
var filenameExtAllowedRe = regexp.MustCompile(`^[a-z0-9]{2,8}$`)

func sanitizeUnsafeFilename(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}

	v = filenameUrlNoiseRe.ReplaceAllString(v, " ")
	v = filenameUnsafeRe.ReplaceAllString(v, "")
	v = filenameSpaceRe.ReplaceAllString(v, " ")
	v = filenameUnderscoreRe.ReplaceAllString(v, "_")
	v = strings.TrimSpace(v)
	v = strings.Trim(v, " ._-")
	return v
}

func normalizeDownAriaTag(v string) string {
	if v == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(v), "[downaria") {
		v = downAriaTagRe.ReplaceAllString(v, "[DownAria]")
	}
	return v
}

func stripDownloadLabelSuffix(filename string) string {
	v := strings.TrimSpace(filename)
	for {
		next := strings.TrimSpace(filenameLabelSuffixRe.ReplaceAllString(v, ""))
		if next == v {
			break
		}
		v = next
	}

	// Keep square brackets to preserve branded suffix like "[DownAria]".
	v = strings.Trim(v, " ._-")
	return strings.TrimSpace(v)
}

func isYouTubeURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "youtube.com" ||
		host == "www.youtube.com" ||
		host == "m.youtube.com" ||
		host == "music.youtube.com" ||
		host == "youtu.be"
}

func isAudioOnlyRequest(req mergeRequest) bool {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	quality := strings.ToLower(strings.TrimSpace(req.Quality))
	return format == "mp3" || format == "m4a" || strings.Contains(quality, "mp3") || strings.Contains(quality, "m4a")
}

func resolveAudioOutput(format, quality string) (ext, codec, contentType string) {
	v := strings.ToLower(strings.TrimSpace(format))
	q := strings.ToLower(strings.TrimSpace(quality))

	if v == "m4a" || strings.Contains(q, "m4a") {
		return "m4a", "aac", "audio/mp4"
	}

	return "mp3", "libmp3lame", "audio/mpeg"
}

func buildYTDLPFormatSelector(quality string, audioOnly bool) string {
	if audioOnly {
		return "bestaudio"
	}

	height := parseQualityHeight(quality)
	if height > 0 {
		return fmt.Sprintf("bestvideo[vcodec^=avc1][height<=%d]+bestaudio/bestvideo[vcodec^=h264][height<=%d]+bestaudio/bestvideo[height<=%d]+bestaudio", height, height, height)
	}

	return "bestvideo[vcodec^=avc1]+bestaudio/bestvideo[vcodec^=h264]+bestaudio/bestvideo+bestaudio"
}

var qualityHeightRe = regexp.MustCompile(`(?i)(\d{3,4})\s*p`)
var filenameLabelSuffixRe = regexp.MustCompile(`(?i)(?:[\s._-]|\(|\[)*(hd|sd|audio|original)(?:\)|\])*$`)

func parseQualityHeight(quality string) int {
	v := strings.TrimSpace(quality)
	if v == "" {
		return 0
	}

	if m := qualityHeightRe.FindStringSubmatch(v); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			return n
		}
	}

	if n, err := strconv.Atoi(v); err == nil {
		return n
	}

	return 0
}
