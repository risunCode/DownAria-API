package handlers

import "net/http"

// HLSStream handles HLS playlist and segment proxy requests.
func (h *Handler) HLSStream(w http.ResponseWriter, r *http.Request) {
	routePath := "/api/v1/hls-stream"
	if len(r.URL.Path) > 0 && r.URL.Path == "/api/web/hls-stream" {
		routePath = "/api/web/hls-stream"
	}
	h.HandleHLSRequest(w, r, routePath)
}
