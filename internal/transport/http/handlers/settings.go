package handlers

import (
	"net/http"

	"downaria-api/pkg/response"
)

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccessRequest(w, r, http.StatusOK, map[string]any{
		"public_base_url":          h.config.PublicBaseURL,
		"merge_enabled":            h.config.MergeEnabled,
		"upstream_timeout_ms":      h.config.UpstreamTimeoutMS,
		"global_rate_limit_limit":  h.config.GlobalRateLimitLimit,
		"global_rate_limit_window": h.config.GlobalRateLimitWindow.String(),
		"global_rate_limit_rule":   h.config.GlobalRateLimitRule,
		"allowed_origins":          h.config.AllowedOrigins,
		"max_download_size_mb":     h.config.MaxDownloadSizeMB,
	})
}
