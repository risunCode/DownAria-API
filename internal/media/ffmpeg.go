package media

import (
	"context"
	"encoding/json"
	"downaria-api/internal/extract"
	runtime "downaria-api/internal/runtime"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func (s *Merger) runFFmpegMerge(ctx context.Context, videoPath, audioPath, format, outputPath string) ([]byte, error) {
	var output []byte
	for attempt := 0; attempt < 2; attempt++ {
		_ = os.Remove(outputPath)
		cmd := exec.CommandContext(ctx, s.ffmpegPath, buildFFmpegArgs(videoPath, audioPath, format, outputPath)...)
		cmd.Env = runtime.MinimalEnv()
		output, _ = cmd.CombinedOutput()
		if cmd.ProcessState != nil && cmd.ProcessState.Success() {
			return output, nil
		}
		if attempt == 0 && s != nil && s.logger != nil {
			s.logger.Warn("ffmpeg first attempt failed, retrying", "output", strings.TrimSpace(string(output)))
		}
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		trimmed = "ffmpeg merge failed"
	}
	return nil, extract.WrapCode(extract.KindMergeFailed, "merge_command_failed", trimmed, false, nil)
}
func (s *Merger) verifyMediaArtifact(ctx context.Context, path string, expectVideo bool, expectAudio bool) error {
	if strings.TrimSpace(path) == "" || s.ffprobePath == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, s.ffprobePath, "-v", "error", "-show_entries", "stream=codec_type", "-of", "json", path)
	cmd.Env = runtime.MinimalEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			trimmed = "ffprobe validation failed"
		}
		return extract.WrapCode(extract.KindMergeFailed, "merge_validation_failed", trimmed, false, err)
	}
	var probe ffprobeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return extract.WrapCode(extract.KindMergeFailed, "merge_validation_invalid_output", "ffprobe returned invalid output", false, err)
	}
	return validateProbeStreams(probe, expectVideo, expectAudio)
}

func validateProbeStreams(probe ffprobeResult, expectVideo bool, expectAudio bool) error {
	var hasVideo, hasAudio bool
	for _, stream := range probe.Streams {
		switch strings.TrimSpace(stream.CodecType) {
		case "video":
			hasVideo = true
		case "audio":
			hasAudio = true
		}
	}
	if expectVideo && !hasVideo {
		return extract.WrapCode(extract.KindMergeFailed, "merge_validation_missing_video", "merged artifact is missing a video stream", false, nil)
	}
	if expectAudio && !hasAudio {
		return extract.WrapCode(extract.KindMergeFailed, "merge_validation_missing_audio", "merged artifact is missing an audio stream", false, nil)
	}
	return nil
}

func (s *Merger) validateRequestedQuality(ctx context.Context, path string, requestedQuality string) error {
	targetHeight := parseTargetHeight(requestedQuality)
	if targetHeight <= 0 || strings.TrimSpace(path) == "" || s.ffprobePath == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, s.ffprobePath, "-v", "error", "-show_entries", "stream=codec_type,width,height", "-of", "json", path)
	cmd.Env = runtime.MinimalEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	var probe ffprobeResult
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil
	}
	actualHeight := 0
	for _, stream := range probe.Streams {
		if strings.TrimSpace(stream.CodecType) == "video" && stream.Height > actualHeight {
			actualHeight = stream.Height
		}
	}
	if actualHeight > 0 && actualHeight+16 < targetHeight {
		return extract.WrapCode(extract.KindMergeFailed, "merge_quality_mismatch", fmt.Sprintf("requested quality %q but output height was %dp", strings.TrimSpace(requestedQuality), actualHeight), false, nil)
	}
	return nil
}

func buildFFmpegArgs(videoPath, audioPath, format, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	args = append(args, "-i", videoPath, "-i", audioPath, "-c:v", "copy", "-c:a", "copy", "-map", "0:v:0", "-map", "1:a:0")
	if format == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, "-f", format, outputPath)
	return args
}

func (s *Merger) runFFmpegContainerConvert(ctx context.Context, inputPath, format, outputPath string) ([]byte, error) {
	var output []byte
	for attempt := 0; attempt < 2; attempt++ {
		_ = os.Remove(outputPath)
		cmd := exec.CommandContext(ctx, s.ffmpegPath, buildFFmpegContainerConvertArgs(inputPath, format, outputPath)...)
		cmd.Env = runtime.MinimalEnv()
		output, _ = cmd.CombinedOutput()
		if cmd.ProcessState != nil && cmd.ProcessState.Success() {
			return output, nil
		}
		if attempt == 0 && s != nil && s.logger != nil {
			s.logger.Warn("ffmpeg container convert first attempt failed, retrying", "output", strings.TrimSpace(string(output)))
		}
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		trimmed = "ffmpeg container convert failed"
	}
	return nil, extract.WrapCode(extract.KindMergeFailed, "merge_container_convert_failed", trimmed, false, nil)
}

func buildFFmpegContainerConvertArgs(inputPath, format, outputPath string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-i", inputPath, "-map", "0:v:0", "-map", "0:a:0?", "-c:v", "copy", "-c:a", "aac"}
	if format == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, "-f", format, outputPath)
	return args
}
