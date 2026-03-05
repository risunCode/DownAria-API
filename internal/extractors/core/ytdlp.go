package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

const (
	ytdlpDumpTimeout      = 30 * time.Second
	ytdlpGetURLsTimeout   = 35 * time.Second
	ytdlpMaxDumpStdout    = 8 << 20
	ytdlpMaxURLsStdout    = 256 << 10
	ytdlpMaxCommandStderr = 64 << 10
)

// YTDLPDumpJSON represents yt-dlp --dump-json output.
type YTDLPDumpJSON struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	Uploader     string           `json:"uploader"`
	UploaderID   string           `json:"uploader_id"`
	UploadDate   string           `json:"upload_date"`
	Duration     float64          `json:"duration"`
	ViewCount    int64            `json:"view_count"`
	LikeCount    int64            `json:"like_count"`
	CommentCount int64            `json:"comment_count"`
	Thumbnail    string           `json:"thumbnail"`
	Thumbnails   []YTDLPThumbnail `json:"thumbnails"`
	Formats      []YTDLPFormat    `json:"formats"`
	URL          string           `json:"url"`
	Ext          string           `json:"ext"`
	Width        int              `json:"width"`
	Height       int              `json:"height"`
	Extractor    string           `json:"extractor"`
	WebpageURL   string           `json:"webpage_url"`
}

type YTDLPThumbnail struct {
	URL string `json:"url"`
}

type YTDLPFormat struct {
	FormatID       string  `json:"format_id"`
	FormatNote     string  `json:"format_note"`
	Quality        float64 `json:"quality"`
	MimeType       string  `json:"mime_type"`
	Ext            string  `json:"ext"`
	URL            string  `json:"url"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox float64 `json:"filesize_approx"`
	TBR            float64 `json:"tbr"`
	ABR            float64 `json:"abr"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Resolution     string  `json:"resolution"`
	Protocol       string  `json:"protocol"`
}

func RunYtDlpDump(ctx context.Context, targetURL string, extraArgs ...string) (*YTDLPDumpJSON, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"--dump-json", "--no-download", "--no-warnings", "--no-playlist", "--js-runtimes", "node"}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	runCtx, cancel := context.WithTimeout(ctx, ytdlpDumpTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "yt-dlp", args...)
	stdout := NewBoundedBuffer(ytdlpMaxDumpStdout)
	stderr := NewBoundedBuffer(ytdlpMaxCommandStderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp execution failed: %w: %s", err, stderr.String())
	}

	var meta YTDLPDumpJSON
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := dec.Decode(&meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func RunYtDlpGetURLs(ctx context.Context, targetURL, formatSelector string, extraArgs ...string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"-g", "--no-warnings", "--no-playlist", "--js-runtimes", "node"}
	if strings.TrimSpace(formatSelector) != "" {
		args = append(args, "-f", strings.TrimSpace(formatSelector))
	}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	runCtx, cancel := context.WithTimeout(ctx, ytdlpGetURLsTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "yt-dlp", args...)
	stdout := NewBoundedBuffer(ytdlpMaxURLsStdout)
	stderr := NewBoundedBuffer(ytdlpMaxCommandStderr)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp url resolve failed: %w: %s", err, stderr.String())
	}

	lines := strings.Split(stdout.String(), "\n")
	urls := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		urls = append(urls, line)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("yt-dlp did not return stream urls")
	}

	return urls, nil
}

type BoundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func NewBoundedBuffer(limit int) *BoundedBuffer {
	if limit <= 0 {
		limit = 1
	}
	return &BoundedBuffer{limit: limit}
}

func (b *BoundedBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
		} else {
			_, _ = b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *BoundedBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buf.Bytes()
}

func (b *BoundedBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

var _ io.Writer = (*BoundedBuffer)(nil)
