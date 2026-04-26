package ytdlp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"downaria-api/internal/extract"
	runtime "downaria-api/internal/runtime"
)

type Output struct{ Stdout, Stderr []byte }

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (Output, error)
}

type ExecRunner struct{}

// Extractor extracts media from URLs using yt-dlp.
type Extractor struct {
	binaryPath string
	runner     Runner
}

type CommandOptions struct{ CookieFile string }

type cookiePair struct{ name, value string }

const (
	// maxCommandOutputBytes caps yt-dlp stdout/stderr at 8MB to prevent memory exhaustion from unexpectedly large output.
	maxCommandOutputBytes = 8 << 20
	// maxCookieFileBytes caps cookie file size at 16KB to limit exposure of sensitive credential data.
	maxCookieFileBytes = 16 << 10
)

// NewExtractor creates a new yt-dlp extractor with the given binary path and runner.
func NewExtractor(binaryPath string, runner Runner) *Extractor {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = "yt-dlp"
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Extractor{binaryPath: binaryPath, runner: runner}
}

// Run executes yt-dlp with the given arguments and returns its output.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (Output, error) {
	if err := validateBinaryPath(name); err != nil {
		return Output{}, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := newLimitedBuffer(maxCommandOutputBytes)
	stderr := newLimitedBuffer(maxCommandOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = runtime.MinimalEnv()
	err := cmd.Run()
	if stdout.overflowed || stderr.overflowed {
		return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, extract.Wrap(extract.KindExtractionFailed, "yt-dlp output exceeded safe limit", nil)
	}
	return Output{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// Extract extracts media metadata from a URL using yt-dlp.
func (e *Extractor) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cookieFile, err := writeCookieFile(opts.CookieHeader, rawURL)
	if err != nil {
		return nil, extract.Wrap(extract.KindInternal, "failed to prepare cookie file", err)
	}
	defer removeCookieFile(cookieFile)
	output, err := e.runner.Run(ctx, e.binaryPath, buildArgs(rawURL, CommandOptions{CookieFile: cookieFile})...)
	if err != nil {
		stderr := strings.TrimSpace(string(output.Stderr))
		if appErr := classifyCommandError(ctx, stderr, err); appErr != nil {
			return nil, appErr
		}
		if isUnsupportedError(stderr, err) {
			return nil, extract.Wrap(extract.KindUnsupportedPlatform, "platform is not supported", err)
		}
		if stderr == "" {
			stderr = "yt-dlp command failed"
		}
		return nil, extract.Wrap(extract.KindExtractionFailed, stderr, err)
	}
	dump, err := DecodeDump(output.Stdout)
	if err != nil {
		return nil, extract.Wrap(extract.KindExtractionFailed, "invalid yt-dlp output", err)
	}
	return MapResult(rawURL, dump)
}

func buildArgs(rawURL string, opts CommandOptions) []string {
	args := []string{"--ignore-config", "--no-config-locations", "--dump-single-json", "--no-warnings", "--no-playlist", "--skip-download", "--restrict-filenames", "--socket-timeout", "20", "--extractor-retries", "2", "--retry-sleep", "extractor:1", "--", rawURL}
	if opts.CookieFile != "" {
		args = append(args[:len(args)-2], "--cookies", opts.CookieFile, "--", rawURL)
	}
	return args
}

func classifyCommandError(ctx context.Context, stderr string, err error) *extract.AppError {
	lower := strings.ToLower(strings.TrimSpace(stderr))
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout") {
		return extract.WrapCode(extract.KindTimeout, "ytdlp_timeout", safeCommandMessage(stderr, "yt-dlp timed out"), true, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return extract.WrapCode(extract.KindTimeout, "ytdlp_cancelled", safeCommandMessage(stderr, "yt-dlp was cancelled"), true, err)
	}
	if isAuthError(lower) {
		return extract.WrapCode(extract.KindAuthRequired, "ytdlp_auth_required", safeCommandMessage(stderr, extract.ErrMsgAuthRequired), false, err)
	}
	if isUpstreamError(lower) {
		return extract.WrapCode(extract.KindUpstreamFailure, "ytdlp_upstream_failure", safeCommandMessage(stderr, extract.ErrMsgUpstreamFailure), true, err)
	}
	return nil
}

func isAuthError(stderr string) bool {
	patterns := []string{"sign in to confirm", "sign in to confirm your age", "login required", "authentication required", "this video is private", "members only", "private video"}
	for _, pattern := range patterns {
		if strings.Contains(stderr, pattern) {
			return true
		}
	}
	return false
}

func isUpstreamError(stderr string) bool {
	patterns := []string{"unable to download webpage", "unable to download api page", "failed to download", "failed to establish a new connection", "connection refused", "connection reset", "remote end closed connection", "temporarily unavailable", "temporary failure", "name or service not known", "no such host", "429: too many requests", "http error 429", "http error 500", "http error 502", "http error 503", "http error 504"}
	for _, pattern := range patterns {
		if strings.Contains(stderr, pattern) {
			return true
		}
	}
	return false
}

func safeCommandMessage(stderr string, fallback string) string {
	trimmed := strings.TrimSpace(stderr)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func writeCookieFile(cookieHeader string, rawURL string) (string, error) {
	if cookieHeader == "" {
		return "", nil
	}
	cookieHeader = sanitizeCookieHeader(cookieHeader)
	if cookieHeader == "" {
		return "", fmt.Errorf("cookie header is empty after sanitization")
	}
	cookieRoot, err := runtime.EnsureSubdir("runtime")
	if err != nil {
		return "", fmt.Errorf("create ytdlp runtime dir: %w", err)
	}
	file, err := os.CreateTemp(cookieRoot, "ytdlp-cookies-*.txt")
	if err != nil {
		return "", fmt.Errorf("create cookie file: %w", err)
	}
	_ = os.Chmod(file.Name(), 0o600)
	domain := cookieDomain(rawURL)
	if domain == "" {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("unable to derive cookie domain")
	}
	content := "# Netscape HTTP Cookie File\n"
	for _, part := range splitCookies(cookieHeader) {
		content += fmt.Sprintf("%s\tTRUE\t/\tTRUE\t0\t%s\t%s\n", domain, part.name, part.value)
		if len(content) > maxCookieFileBytes {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return "", fmt.Errorf("cookie file exceeds safe limit")
		}
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("write cookie file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("close cookie file: %w", err)
	}
	return file.Name(), nil
}

func removeCookieFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func splitCookies(header string) []cookiePair {
	parts := bytes.Split([]byte(header), []byte(";"))
	out := make([]cookiePair, 0, len(parts))
	for _, part := range parts {
		pair := bytes.SplitN(bytes.TrimSpace(part), []byte("="), 2)
		if len(pair) == 2 && len(pair[0]) > 0 {
			name := sanitizeCookieToken(string(bytes.TrimSpace(pair[0])))
			value := sanitizeCookieValue(string(bytes.TrimSpace(pair[1])))
			if name == "" || value == "" {
				continue
			}
			out = append(out, cookiePair{name: name, value: value})
		}
	}
	return out
}
