package handlers

import (
	"net/http"
	"time"

	"downaria-api/pkg/response"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccessRequest(w, r, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
