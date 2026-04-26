package media

import (
	"context"
	"encoding/json"
	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	logging "downaria-api/internal/logging"
	runtime "downaria-api/internal/runtime"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"os/exec"
	"strings"
)

type ConvertRequest struct {
	URL          string
	Filename     string
	Format       string
	AudioOnly    bool
	UserAgent    string
	CookieHeader string
}

type ConvertResult struct {
	FilePath     string
	Filename     string
	ContentType  string
	ContentBytes int64
}

type Converter struct {
	ffmpegPath  string
	ffprobePath string
	logger      *slog.Logger
}

func NewConverter(loggers ...*slog.Logger) *Converter {
	path := ResolveBinary("", "ffmpeg", "ffmpeg.exe")
	probePath := ResolveBinary("", "ffprobe", "ffprobe.exe")
	return &Converter{ffmpegPath: path, ffprobePath: probePath, logger: logging.FallbackLogger(loggers...)}
}

func (s *Converter) Convert(ctx context.Context, req ConvertRequest) (*ConvertResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.URL) == "" {
		return nil, extract.WrapCode(extract.KindInvalidInput, "convert_url_required", "url is required", false, nil)
	}
	if s.ffmpegPath == "" {
		return nil, extract.WrapCode(extract.KindInternal, "ffmpeg_unavailable", "ffmpeg is unavailable", false, nil)
	}
	s.logInfo(ctx, "convert_stage", slog.String("stage", "start"), slog.String("url", req.URL), slog.String("format", strings.TrimSpace(req.Format)))
	format := normalizeConvertFormat(req.Format, req.AudioOnly)
	filename := sanitizeConvertOutputName(req.Filename, format)
	convertRoot, err := runtime.EnsureSubdir("conversions")
	if err != nil {
		return nil, extract.WrapCode(extract.KindInternal, "convert_output_create_failed", "failed to create output file", false, err)
	}
	file, err := os.CreateTemp(convertRoot, "convert-*")
	if err != nil {
		return nil, extract.WrapCode(extract.KindInternal, "convert_output_create_failed", "failed to create output file", false, err)
	}
	outputPath := file.Name() + "." + format
	_ = file.Close()
	_ = os.Remove(file.Name())
	args := buildConvertFFmpegArgs(req, format, outputPath)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)
	cmd.Env = runtime.MinimalEnv()
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(outputPath)
		s.logWarn(ctx, "convert_stage", slog.String("stage", "command_failed"), slog.String("url", req.URL), slog.String("error", strings.TrimSpace(string(output))))
		return nil, extract.WrapCode(extract.KindConvertFailed, "convert_command_failed", strings.TrimSpace(string(output)), false, err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, extract.WrapCode(extract.KindInternal, "convert_output_missing", "converted output not found", false, err)
	}
	if err := s.verifyConvertedOutput(ctx, outputPath, req.AudioOnly || isAudioFormat(format)); err != nil {
		_ = os.Remove(outputPath)
		return nil, err
	}
	s.logInfo(ctx, "convert_stage", slog.String("stage", "completed"), slog.String("url", req.URL), slog.String("format", format), slog.Int64("bytes", info.Size()))
	return &ConvertResult{FilePath: outputPath, Filename: filename, ContentType: convertContentTypeFromFormat(format), ContentBytes: info.Size()}, nil
}

func (s *Converter) verifyConvertedOutput(ctx context.Context, path string, audioOnly bool) error {
	if s == nil || s.ffprobePath == "" || strings.TrimSpace(path) == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, s.ffprobePath, "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", path)
	cmd.Env = runtime.MinimalEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return extract.WrapCode(extract.KindConvertFailed, "convert_validation_failed", strings.TrimSpace(string(output)), false, err)
	}
	var probe convertProbeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return extract.WrapCode(extract.KindConvertFailed, "convert_validation_failed", "invalid ffprobe output", false, err)
	}
	return validateConvertProbeStreams(probe, audioOnly)
}

func validateConvertProbeStreams(probe convertProbeResult, audioOnly bool) error {
	hasAudio := false
	hasVideo := false
	for _, stream := range probe.Streams {
		switch strings.TrimSpace(stream.CodecType) {
		case "audio":
			hasAudio = true
		case "video":
			hasVideo = true
		}
	}
	if audioOnly {
		if !hasAudio {
			return extract.WrapCode(extract.KindConvertFailed, "convert_validation_missing_audio", "converted output is missing audio", false, nil)
		}
		return nil
	}
	if !hasVideo {
		return extract.WrapCode(extract.KindConvertFailed, "convert_validation_missing_video", "converted output is missing video", false, nil)
	}
	return nil
}

func isAudioFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "mp3", "m4a", "aac", "opus", "ogg", "wav", "flac":
		return true
	default:
		return false
	}
}

func buildConvertFFmpegArgs(req ConvertRequest, format, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if ua := strings.TrimSpace(req.UserAgent); ua != "" {
		args = append(args, "-user_agent", ua)
	}
	if cookie := strings.TrimSpace(req.CookieHeader); cookie != "" {
		args = append(args, "-headers", fmt.Sprintf("Cookie: %s\r\n", cookie))
	}
	args = append(args, "-i", req.URL)
	if req.AudioOnly || format == "mp3" || format == "m4a" || format == "aac" || format == "opus" {
		args = append(args, "-vn")
		switch format {
		case "m4a", "aac":
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		case "opus":
			args = append(args, "-c:a", "libopus", "-b:a", "128k")
		default:
			args = append(args, "-c:a", "libmp3lame", "-q:a", "0")
		}
	} else {
		switch format {
		case "webm":
			args = append(args, "-c:v", "libvpx-vp9", "-c:a", "libopus")
		case "mkv":
			args = append(args, "-c", "copy")
		default:
			args = append(args, "-c:v", "copy", "-c:a", "aac", "-movflags", "+faststart")
		}
	}
	args = append(args, "-f", ffmpegMuxerForFormat(format), outputPath)
	return args
}

func normalizeConvertFormat(value string, audioOnly bool) string {
	format := strings.ToLower(strings.TrimSpace(value))
	if format == "" {
		if audioOnly {
			return "mp3"
		}
		return "mp4"
	}
	return format
}

func sanitizeConvertOutputName(name string, format string) string {
	base := extract.SanitizeFilename(name, "converted")
	return extract.EnsureFilenameExtension(base, format)
}

func convertContentTypeFromFormat(format string) string {
	if value := mime.TypeByExtension("." + format); value != "" {
		return value
	}
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "m4a", "aac":
		return "audio/mp4"
	case "opus":
		return "audio/ogg"
	case "webm":
		return "video/webm"
	case "mkv":
		return "video/x-matroska"
	default:
		return "video/mp4"
	}
}

func ffmpegMuxerForFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "m4a":
		return "ipod"
	case "aac":
		return "adts"
	default:
		return strings.ToLower(strings.TrimSpace(format))
	}
}
func (s *Converter) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if s == nil || s.logger == nil {
		return
	}
	attrs = append([]slog.Attr{slog.String("request_id", middleware.FromContext(ctx))}, attrs...)
	s.logger.LogAttrs(ctx, level, message, attrs...)
}

func (s *Converter) logInfo(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelInfo, msg, attrs...)
}

func (s *Converter) logWarn(ctx context.Context, msg string, attrs ...slog.Attr) {
	s.log(ctx, slog.LevelWarn, msg, attrs...)
}
