package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

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

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &FFmpegResult{
		Stdout: stdout,
		Cmd:    cmd,
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
	if codec == "" {
		codec = "libmp3lame"
	}

	format := strings.ToLower(strings.TrimSpace(opts.OutputExt))
	if format == "" {
		format = "mp3"
	}

	args = append(args,
		"-i", opts.InputURL,
		"-vn",
		"-c:a", codec,
	)

	if codec == "aac" {
		args = append(args, "-b:a", "192k")
	} else {
		args = append(args, "-q:a", "0")
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

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &FFmpegResult{Stdout: stdout, Cmd: cmd}, nil
}

type FFmpegResult struct {
	Stdout io.ReadCloser
	Cmd    *exec.Cmd
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
	return r.Cmd.Wait()
}
