package probe

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type Getter interface {
	Get(context.Context, string, map[string]string) (*http.Response, error)
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type GetterFunc func(context.Context, string, map[string]string) (*http.Response, error)

func (f GetterFunc) Get(ctx context.Context, rawURL string, headers map[string]string) (*http.Response, error) {
	return f(ctx, rawURL, headers)
}

func SizeWithGetter(ctx context.Context, getter Getter, rawURL string, headers map[string]string) int64 {
	if getter == nil || strings.TrimSpace(rawURL) == "" {
		return 0
	}
	probeHeaders := cloneHeaders(headers)
	probeHeaders["Range"] = "bytes=0-0"
	resp, err := getter.Get(ctx, rawURL, probeHeaders)
	if err != nil || resp == nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1))
	if size := sizeFromResponse(resp); size > 1 {
		return size
	}
	return 0
}

func SizeWithDoer(ctx context.Context, doer Doer, rawURL string, headers map[string]string) int64 {
	if doer == nil || strings.TrimSpace(rawURL) == "" {
		return 0
	}
	if size := sizeFromDoer(ctx, doer, http.MethodHead, rawURL, headers); size > 1 {
		return size
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0
	}
	for key, value := range cloneHeaders(headers) {
		req.Header.Set(key, value)
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := doer.Do(req)
	if err != nil || resp == nil {
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1))
	if size := sizeFromResponse(resp); size > 1 {
		return size
	}
	return 0
}

func sizeFromDoer(ctx context.Context, doer Doer, method, rawURL string, headers map[string]string) int64 {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return 0
	}
	for key, value := range cloneHeaders(headers) {
		req.Header.Set(key, value)
	}
	resp, err := doer.Do(req)
	if err != nil || resp == nil {
		return 0
	}
	defer resp.Body.Close()
	return sizeFromResponse(resp)
}

func sizeFromResponse(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if value := strings.TrimSpace(resp.Header.Get("Content-Length")); value != "" {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 1 {
			return n
		}
	}
	contentRange := strings.TrimSpace(resp.Header.Get("Content-Range"))
	if idx := strings.LastIndex(contentRange, "/"); idx >= 0 && idx+1 < len(contentRange) {
		if n, err := strconv.ParseInt(strings.TrimSpace(contentRange[idx+1:]), 10, 64); err == nil && n > 1 {
			return n
		}
	}
	return 0
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			cloned[key] = value
		}
	}
	return cloned
}
