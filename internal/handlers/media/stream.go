package media

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"fetchmoona/internal/infra/network"
	"fetchmoona/internal/shared/util"
	"fetchmoona/pkg/response"
)

type StreamHandler struct {
	streamer *network.Streamer
}

func NewStreamHandler() *StreamHandler {
	return &StreamHandler{
		streamer: network.NewStreamer(),
	}
}

func (h *StreamHandler) Handle(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		response.WriteError(w, http.StatusBadRequest, "MISSING_URL", "URL parameter is required")
		return
	}

	headers := buildStreamHeaders(url)
	rangeHeader := r.Header.Get("Range")

	result, err := h.streamer.Stream(r.Context(), network.StreamOptions{
		URL:         url,
		Headers:     headers,
		RangeHeader: rangeHeader,
	})

	if err != nil {
		response.WriteError(w, http.StatusBadGateway, "STREAM_FAILED", err.Error())
		return
	}
	defer result.Body.Close()

	// Copy headers
	w.Header().Set("Content-Type", result.ContentType)
	if result.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", result.ContentLength))
	}
	if cr := result.Headers["Content-Range"]; cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	w.WriteHeader(result.StatusCode)
	_, _ = io.Copy(w, result.Body)
}

// buildStreamHeaders builds platform-specific headers for streaming
func buildStreamHeaders(targetURL string) map[string]string {
	headers := make(map[string]string)
	headers["User-Agent"] = util.DefaultUserAgent

	lowerURL := strings.ToLower(targetURL)
	switch {
	case strings.Contains(lowerURL, "facebook.com") || strings.Contains(lowerURL, "fbcdn.net"):
		headers["Referer"] = "https://www.facebook.com/"
	case strings.Contains(lowerURL, "instagram.com") || strings.Contains(lowerURL, "cdninstagram.com"):
		headers["Referer"] = "https://www.instagram.com/"
	case strings.Contains(lowerURL, "googlevideo.com") || strings.Contains(lowerURL, "youtube.com"):
		headers["Referer"] = "https://www.youtube.com/"
		headers["Origin"] = "https://www.youtube.com"
	case strings.Contains(lowerURL, "pixiv.net") || strings.Contains(lowerURL, "pximg.net"):
		headers["Referer"] = "https://www.pixiv.net/"
	}

	return headers
}
