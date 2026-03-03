package handlers

import "fetchmoona/internal/shared/security"

func redactLogValue(value string) string {
	return security.RedactLogValue(value)
}

func redactLogError(err error) string {
	return security.RedactLogError(err)
}
