package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	apperrors "fetchmoona/internal/core/errors"
)

func TestRequireWebSignature_BodyTooLarge(t *testing.T) {
	secret := "top-secret"
	body := bytes.Repeat([]byte("a"), webSigMaxBodyBytes+1)

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := "nonce-oversized"
	sig := buildWebSignature(secret, http.MethodPost, "/api/web/extract", ts, nonce, body)

	mw := RequireWebSignature(secret)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/web/extract", bytes.NewReader(body))
	req.Header.Set(webSigHeaderTimestamp, ts)
	req.Header.Set(webSigHeaderNonce, nonce)
	req.Header.Set(webSigHeaderSignature, sig)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != apperrors.HTTPStatus(apperrors.CodeFileTooLarge) {
		t.Fatalf("expected status %d, got %d", apperrors.HTTPStatus(apperrors.CodeFileTooLarge), rec.Code)
	}
}

func TestRequireWebSignature_ValidSmallBody(t *testing.T) {
	secret := "top-secret"
	body := []byte(`{"url":"https://example.com"}`)

	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	nonce := "nonce-valid"
	sig := buildWebSignature(secret, http.MethodPost, "/api/web/extract", ts, nonce, body)

	mw := RequireWebSignature(secret)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/web/extract", bytes.NewReader(body))
	req.Header.Set(webSigHeaderTimestamp, ts)
	req.Header.Set(webSigHeaderNonce, nonce)
	req.Header.Set(webSigHeaderSignature, sig)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireWebSignature_HeaderContractNames(t *testing.T) {
	if got, want := webSigHeaderTimestamp, "X-Downaria-Timestamp"; got != want {
		t.Fatalf("timestamp header contract changed: got %q want %q", got, want)
	}
	if got, want := webSigHeaderNonce, "X-Downaria-Nonce"; got != want {
		t.Fatalf("nonce header contract changed: got %q want %q", got, want)
	}
	if got, want := webSigHeaderSignature, "X-Downaria-Signature"; got != want {
		t.Fatalf("signature header contract changed: got %q want %q", got, want)
	}
}
