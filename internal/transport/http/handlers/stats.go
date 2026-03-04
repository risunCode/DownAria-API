package handlers

import (
	"net/http"
	"time"

	"downaria-api/internal/shared/util"
	"downaria-api/pkg/response"
)

func (h *Handler) PublicStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	clientIP := ""
	if h.clientIPFn != nil {
		clientIP = h.clientIPFn(r)
	} else {
		clientIP = util.ClientIPFromRequest(r)
	}
	h.statsStore.RecordVisitor(clientIP, now)
	snapshot := h.statsStore.Snapshot(now)

	response.WriteSuccessRequest(w, r, http.StatusOK, snapshot)
}
