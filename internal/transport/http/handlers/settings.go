package handlers

import (
	"net/http"

	"downaria-api/pkg/response"
)

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccessRequest(w, r, http.StatusOK, map[string]any{
		"public_base_url":                     h.config.PublicBaseURL,
		"merge_enabled":                       h.config.MergeEnabled,
		"streaming_download_enabled":          h.config.StreamingDownloadEnabled,
		"concurrent_merge_enabled":            h.config.ConcurrentMergeEnabled,
		"hls_streaming_enabled":               h.config.HLSStreamingEnabled,
		"hls_merge_enabled":                   h.config.HLSMergeEnabled,
		"upstream_timeout_ms":                 h.config.UpstreamTimeoutMS,
		"upstream_connect_timeout_ms":         h.config.UpstreamConnectTimeoutMS,
		"upstream_tls_handshake_timeout_ms":   h.config.UpstreamTLSHandshakeTimeoutMS,
		"upstream_response_header_timeout_ms": h.config.UpstreamResponseHeaderTimeoutMS,
		"upstream_idle_conn_timeout_ms":       h.config.UpstreamIdleConnTimeoutMS,
		"upstream_keepalive_timeout_ms":       h.config.UpstreamKeepAliveTimeoutMS,
		"global_rate_limit_limit":             h.config.GlobalRateLimitLimit,
		"global_rate_limit_window":            h.config.GlobalRateLimitWindow.String(),
		"global_rate_limit_rule":              h.config.GlobalRateLimitRule,
		"allowed_origins":                     h.config.AllowedOrigins,
		"max_download_size_mb":                h.config.MaxDownloadSizeMB,
		"merge_worker_count":                  h.config.MergeWorkerCount,
		"hls_segment_worker_count":            h.config.HLSSegmentWorkerCount,
		"hls_segment_max_retries":             h.config.HLSSegmentMaxRetries,
		"buffer_size_video":                   h.config.BufferSizeVideo,
		"buffer_size_audio":                   h.config.BufferSizeAudio,
	})
}
