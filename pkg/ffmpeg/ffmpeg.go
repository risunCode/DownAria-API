package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	extractorcore "downaria-api/internal/extractors/core"
)

const maxFFmpegStderrBytes = 64 << 10

type FFmpeg struct {
	path string
}

type MergeOptions struct {
	VideoURL  string
	AudioURL  string
	UserAgent string
	Headers   map[string]string
}

type AudioExtractOptions struct {
	InputURL   string
	OutputExt  string
	UserAgent  string
	Headers    map[string]string
	AudioCodec string
}

var (
	cachedPath string
	pathOnce   sync.Once
)

func New() *FFmpeg {
	path := GetPath()
	if path == "" {
		return nil
	}
	return &FFmpeg{path: path}
}

func GetPath() string {
	pathOnce.Do(func() {
		cachedPath = detectFFmpegPath()
	})
	return cachedPath
}

func detectFFmpegPath() string {
	if runtime.GOOS == "windows" {
		paths := []string{
			"ffmpeg.exe",
			"C:\\ffmpeg\\bin\\ffmpeg.exe",
			"C:\\Program Files\\ffmpeg\\bin\\ffmpeg.exe",
		}
		for _, p := range paths {
			if _, err := exec.LookPath(p); err == nil {
				path, _ := exec.LookPath(p)
				return path
			}
		}
	} else {
		if path, err := exec.LookPath("ffmpeg"); err == nil {
			return path
		}
	}
	return ""
}

func (f *FFmpeg) StreamMerge(ctx context.Context, opts MergeOptions) (*FFmpegResult, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-user_agent", opts.UserAgent,
	}

	if len(opts.Headers) > 0 {
		var hStr strings.Builder
		for k, v := range opts.Headers {
			hStr.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
		args = append(args, "-headers", hStr.String())
	}

	args = append(args,
		"-i", opts.VideoURL,
		"-i", opts.AudioURL,
		"-c:v", "copy",
		"-c:a", "copy",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof+faststart",
		"-f", "mp4",
		"-",
	)

	cmd := exec.CommandContext(ctx, f.path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := extractorcore.NewBoundedBuffer(maxFFmpegStderrBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &FFmpegResult{
		Stdout: stdout,
		Cmd:    cmd,
		stderr: stderr,
	}, nil
}

func (f *FFmpeg) StreamExtractAudio(ctx context.Context, opts AudioExtractOptions) (*FFmpegResult, error) {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-user_agent", opts.UserAgent,
	}

	if len(opts.Headers) > 0 {
		var hStr strings.Builder
		for k, v := range opts.Headers {
			hStr.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
		args = append(args, "-headers", hStr.String())
	}

	codec := strings.TrimSpace(opts.AudioCodec)
	format := strings.ToLower(strings.TrimSpace(opts.OutputExt))
	if format == "" {
		format = "mp3"
	}

	// Resolve actual audio codec based on format if not specified
	if codec == "" {
		if format == "m4a" || format == "aac" {
			codec = "aac"
		} else if format == "mp3" {
			codec = "libmp3lame"
		} else if format == "opus" {
			codec = "libopus"
		} else {
			codec = "libmp3lame" // fallback
		}
	}

	// Normalize codec names
	codecLower := strings.ToLower(codec)

	args = append(args,
		"-i", opts.InputURL,
		"-vn",
		"-c:a", codec,
	)

	// Set bitrate/quality based on codec
	if codecLower == "aac" || strings.Contains(codecLower, "aac") {
		args = append(args, "-b:a", "192k")
	} else if codecLower == "libopus" || strings.Contains(codecLower, "opus") {
		args = append(args, "-b:a", "128k")
	} else {
		args = append(args, "-q:a", "0") // VBR for mp3
	}

	args = append(args,
		"-f", format,
		"-",
	)

	cmd := exec.CommandContext(ctx, f.path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := extractorcore.NewBoundedBuffer(maxFFmpegStderrBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &FFmpegResult{Stdout: stdout, Cmd: cmd, stderr: stderr}, nil
}

type FFmpegResult struct {
	Stdout io.ReadCloser
	Cmd    *exec.Cmd
	stderr *extractorcore.BoundedBuffer
}

func (r *FFmpegResult) Close() error {
	if r.Stdout != nil {
		r.Stdout.Close()
	}
	if r.Cmd != nil && r.Cmd.Process != nil {
		r.Cmd.Process.Kill()
	}
	return nil
}

func (r *FFmpegResult) Wait() error {
	if r == nil || r.Cmd == nil {
		return nil
	}
	err := r.Cmd.Wait()
	if err == nil {
		return nil
	}
	if r.stderr != nil {
		errText := strings.TrimSpace(r.stderr.String())
		if errText != "" {
			return fmt.Errorf("%w: %s", err, errText)
		}
	}
	return err
}
