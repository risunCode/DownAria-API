package handlers

import (
	"context"

	"downaria-api/internal/shared/security"
)

func (h *Handler) sanitizeAndValidateOutboundURL(ctx context.Context, raw string) (string, error) {
	sanitized, err := security.SanitizeHTTPURL(raw)
	if err != nil {
		return "", err
	}

	guard := h.urlGuard
	if guard == nil {
		guard = security.NewOutboundURLValidator(nil)
	}

	validated, err := guard.Validate(ctx, sanitized.String())
	if err != nil {
		return "", err
	}

	return validated.String(), nil
}
