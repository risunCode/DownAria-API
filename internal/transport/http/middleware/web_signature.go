package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	apperrors "fetchmoona/internal/core/errors"
	"fetchmoona/pkg/response"
)

const (
	webSigHeaderTimestamp = "X-Downaria-Timestamp"
	webSigHeaderNonce     = "X-Downaria-Nonce"
	webSigHeaderSignature = "X-Downaria-Signature"
	webSigMaxClockSkew    = 60 * time.Second
	webSigNonceTTL        = 2 * time.Minute
	webSigMaxBodyBytes    = 1 << 20
)

type nonceStore struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func newNonceStore() *nonceStore {
	return &nonceStore{items: make(map[string]time.Time)}
}

func (s *nonceStore) seenOrStore(nonce string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, exp := range s.items {
		if now.After(exp) {
			delete(s.items, key)
		}
	}

	if _, exists := s.items[nonce]; exists {
		return true
	}

	s.items[nonce] = now.Add(webSigNonceTTL)
	return false
}

func RequireWebSignature(sharedSecret string) func(http.Handler) http.Handler {
	nonces := newNonceStore()
	secret := strings.TrimSpace(sharedSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "WEB_INTERNAL_SHARED_SECRET is missing on API server")
				return
			}

			timestampRaw := strings.TrimSpace(r.Header.Get(webSigHeaderTimestamp))
			nonce := strings.TrimSpace(r.Header.Get(webSigHeaderNonce))
			signature := strings.TrimSpace(r.Header.Get(webSigHeaderSignature))

			if timestampRaw == "" || nonce == "" || signature == "" {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "missing web signature headers")
				return
			}

			ts, err := strconv.ParseInt(timestampRaw, 10, 64)
			if err != nil {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "invalid web signature timestamp")
				return
			}

			now := time.Now().UTC()
			tsTime := time.Unix(ts, 0).UTC()
			skew := now.Sub(tsTime)
			if skew < 0 {
				skew = -skew
			}
			if skew > webSigMaxClockSkew {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "web signature expired")
				return
			}

			if nonces.seenOrStore(nonce, now) {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "web signature replay detected")
				return
			}

			body, err := readBodyForSignature(w, r)
			if err != nil {
				if errors.Is(err, errWebSigBodyTooLarge) {
					response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeFileTooLarge), apperrors.CodeFileTooLarge, apperrors.Message(apperrors.CodeFileTooLarge))
					return
				}
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "failed to verify web signature")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			expected := buildWebSignature(secret, r.Method, r.URL.Path, timestampRaw, nonce, body)
			if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
				response.WriteErrorRequest(w, r, apperrors.HTTPStatus(apperrors.CodeAccessDenied), apperrors.CodeAccessDenied, "invalid web signature")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

var errWebSigBodyTooLarge = errors.New("web signature body too large")

func readBodyForSignature(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}

	if r.ContentLength > webSigMaxBodyBytes {
		return nil, errWebSigBodyTooLarge
	}

	r.Body = http.MaxBytesReader(w, r.Body, webSigMaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errWebSigBodyTooLarge
		}
		return nil, err
	}

	return body, nil
}

func buildWebSignature(secret, method, path, timestamp, nonce string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(path), strings.TrimSpace(timestamp), strings.TrimSpace(nonce), hex.EncodeToString(bodyHash[:]))

	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(canonical))
	return hex.EncodeToString(h.Sum(nil))
}
