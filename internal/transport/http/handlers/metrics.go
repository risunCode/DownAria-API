package handlers

import "net/http"

func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	h.metrics.Handler().ServeHTTP(w, r)
}
