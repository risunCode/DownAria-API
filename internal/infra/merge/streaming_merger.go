package merge

import (
	"context"
	"errors"
	"fmt"
	"io"

	"downaria-api/pkg/ffmpeg"
)

type MergeInput struct {
	VideoURL  string
	AudioURL  string
	UserAgent string
	Headers   map[string]string
}

var ErrOutputLimitExceeded = errors.New("merge output exceeds size limit")

type MergeRunner interface {
	StreamMerge(ctx context.Context, opts ffmpeg.MergeOptions) (*ffmpeg.FFmpegResult, error)
}

type StreamingMerger struct {
	MaxOutputSize int64
	FFmpegPath    string
	runner        MergeRunner
}

func NewStreamingMerger(ffmpegPath string, maxOutputSize int64) *StreamingMerger {
	if maxOutputSize <= 0 {
		maxOutputSize = 512 * 1024 * 1024
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return &StreamingMerger{FFmpegPath: ffmpegPath, MaxOutputSize: maxOutputSize, runner: ffmpeg.New()}
}

func NewStreamingMergerWithRunner(runner MergeRunner, maxOutputSize int64) *StreamingMerger {
	if maxOutputSize <= 0 {
		maxOutputSize = 512 * 1024 * 1024
	}
	return &StreamingMerger{FFmpegPath: "ffmpeg", MaxOutputSize: maxOutputSize, runner: runner}
}

func (m *StreamingMerger) MergeAndStream(ctx context.Context, input *MergeInput, output io.Writer) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || input.VideoURL == "" || input.AudioURL == "" {
		return 0, fmt.Errorf("missing merge input URLs")
	}
	if output == nil {
		return 0, fmt.Errorf("missing merge output writer")
	}
	if m == nil || m.runner == nil {
		return 0, fmt.Errorf("ffmpeg is unavailable")
	}

	result, err := m.runner.StreamMerge(ctx, ffmpeg.MergeOptions{
		VideoURL:  input.VideoURL,
		AudioURL:  input.AudioURL,
		UserAgent: input.UserAgent,
		Headers:   input.Headers,
	})
	if err != nil {
		return 0, err
	}
	defer result.Close()

	if m.MaxOutputSize > 0 {
		written, err := io.Copy(output, io.LimitReader(result.Stdout, m.MaxOutputSize+1))
		if err != nil {
			if ctx.Err() != nil {
				return written, ctx.Err()
			}
			return written, err
		}
		if written > m.MaxOutputSize {
			_ = result.Close()
			_ = result.Wait()
			return written, ErrOutputLimitExceeded
		}
		if err := result.Wait(); err != nil {
			if ctx.Err() != nil {
				return written, ctx.Err()
			}
			return written, err
		}
		return written, nil
	}

	written, err := io.Copy(output, result.Stdout)
	if err != nil {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		return written, err
	}
	if err := result.Wait(); err != nil {
		if ctx.Err() != nil {
			return written, ctx.Err()
		}
		return written, err
	}
	return written, nil
}
