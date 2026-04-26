package api

import (
	"net/http"
	"strings"
	"testing"

	"downaria-api/internal/extract"
)

func TestDecodeExtractRejectsUnknownFields(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"https://example.com","extra":1}`))
	_, err := decodeExtractRequest(r)
	if !extract.IsKind(err, extract.KindInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeMediaValidatesShape(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"https://example.com/a.mp4","video_url":"https://example.com/v.mp4","audio_url":"https://example.com/a.m4a"}`))
	_, err := decodeMediaRequest(r)
	if app := extract.AsAppError(err); app == nil || app.Code != "media_request_ambiguous" {
		t.Fatalf("err = %v", err)
	}
}
