package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

	args := []string{"--dump-json", "--no-download", "--no-warnings", "--no-playlist"}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp execution failed: %w", err)
	}

	var meta YTDLPDumpJSON
	if err := json.Unmarshal(stdout.Bytes(), &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func RunYtDlpGetURLs(ctx context.Context, targetURL, formatSelector string, extraArgs ...string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"-g", "--no-warnings", "--no-playlist"}
	if strings.TrimSpace(formatSelector) != "" {
		args = append(args, "-f", strings.TrimSpace(formatSelector))
	}
	args = append(args, extraArgs...)
	args = append(args, targetURL)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp url resolve failed: %w", err)
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
