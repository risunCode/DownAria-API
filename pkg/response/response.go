package response

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"fetchmoona/internal/shared/util"
)

const (
	RequestIDContextKey       = "requestId"
	LegacyRequestIDContextKey = "request_id"
)

type Envelope struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
	Meta    *Meta     `json:"meta,omitempty"`
}

type APIError struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Category string         `json:"category,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Meta struct {
	RequestID     string         `json:"requestId,omitempty"`
	Timestamp     string         `json:"timestamp,omitempty"`
	ResponseTime  int64          `json:"responseTime,omitempty"`
	Cached        *bool          `json:"cached,omitempty"`
	RateLimit     *RateLimitInfo `json:"rateLimit,omitempty"`
	AccessMode    string         `json:"accessMode,omitempty"`
	PublicContent *bool          `json:"publicContent,omitempty"`
	CookieSource  string         `json:"cookieSource,omitempty"`
}

type RateLimitInfo struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

type Builder struct {
	request   *http.Request
	requestID string
	startTime time.Time
	response  Envelope
}

func NewBuilder() *Builder {
	return &Builder{
		requestID: util.GenerateRequestID(),
		startTime: time.Now().UTC(),
	}
}

func NewBuilderFromRequest(r *http.Request) *Builder {
	b := NewBuilder()
	b.request = r
	b.requestID = resolveRequestID(r)
	return b
}

func (b *Builder) Success(data any) *Builder {
	b.response.Success = true
	b.response.Data = data
	b.response.Error = nil
	return b
}

func (b *Builder) Error(code, message string) *Builder {
	return b.ErrorWithDetails(code, message, "", nil)
}

func (b *Builder) ErrorWithDetails(code, message, category string, metadata map[string]any) *Builder {
	b.response.Success = false
	b.response.Data = nil
	b.response.Error = &APIError{
		Code:     code,
		Message:  message,
		Category: strings.TrimSpace(category),
		Metadata: cloneMetadata(metadata),
	}
	return b
}

func (b *Builder) WithCached(cached bool) *Builder {
	meta := b.ensureMeta()
	meta.Cached = &cached
	return b
}

func (b *Builder) WithRateLimit(limit, remaining int, reset int64) *Builder {
	meta := b.ensureMeta()
	meta.RateLimit = &RateLimitInfo{Limit: limit, Remaining: remaining, Reset: reset}
	return b
}

func (b *Builder) WithAccessMode(accessMode string) *Builder {
	meta := b.ensureMeta()
	meta.AccessMode = accessMode
	return b
}

func (b *Builder) WithPublicContent(publicContent bool) *Builder {
	meta := b.ensureMeta()
	meta.PublicContent = &publicContent
	return b
}

func (b *Builder) WithCookieSource(cookieSource string) *Builder {
	meta := b.ensureMeta()
	meta.CookieSource = cookieSource
	return b
}

func (b *Builder) Build() Envelope {
	meta := b.ensureMeta()
	meta.RequestID = resolveCanonicalRequestID(meta.RequestID, b.requestID)
	if meta.Timestamp == "" {
		meta.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if b.startTime.IsZero() {
		meta.ResponseTime = 0
	} else {
		meta.ResponseTime = time.Since(b.startTime).Milliseconds()
	}
	b.response.Meta = meta
	return b.response
}

func (b *Builder) Write(w http.ResponseWriter, statusCode int) error {
	resp := b.Build()
	if resp.Meta != nil && resp.Meta.RequestID != "" {
		w.Header().Set("X-Request-ID", resp.Meta.RequestID)
	}
	return writeJSON(w, statusCode, resp)
}

func (b *Builder) WriteSuccess(w http.ResponseWriter, data any) error {
	return b.Success(data).Write(w, http.StatusOK)
}

func (b *Builder) WriteError(w http.ResponseWriter, statusCode int, code, message string) error {
	return b.Error(code, message).Write(w, statusCode)
}

func (b *Builder) WriteErrorWithDetails(w http.ResponseWriter, statusCode int, code, message, category string, metadata map[string]any) error {
	return b.ErrorWithDetails(code, message, category, metadata).Write(w, statusCode)
}

func WriteSuccess(w http.ResponseWriter, statusCode int, data any) {
	_ = NewBuilder().Success(data).Write(w, statusCode)
}

func WriteSuccessRequest(w http.ResponseWriter, r *http.Request, statusCode int, data any) {
	_ = NewBuilderFromRequest(r).Success(data).Write(w, statusCode)
}

func WriteSuccessWithMeta(w http.ResponseWriter, statusCode int, data any, meta any) {
	b := NewBuilder().Success(data)
	if m, ok := meta.(map[string]any); ok {
		if rid, ok := m["requestId"].(string); ok {
			b.ensureMeta().RequestID = strings.TrimSpace(rid)
		}
		if rt, ok := m["responseTime"].(int64); ok {
			b.ensureMeta().ResponseTime = rt
		}
	}
	_ = b.Write(w, statusCode)
}

func WriteError(w http.ResponseWriter, statusCode int, code, message string) {
	_ = NewBuilder().Error(code, message).Write(w, statusCode)
}

func WriteErrorWithDetails(w http.ResponseWriter, statusCode int, code, message, category string, metadata map[string]any) {
	_ = NewBuilder().ErrorWithDetails(code, message, category, metadata).Write(w, statusCode)
}

func WriteErrorRequest(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	_ = NewBuilderFromRequest(r).Error(code, message).Write(w, statusCode)
}

func WriteErrorRequestWithDetails(w http.ResponseWriter, r *http.Request, statusCode int, code, message, category string, metadata map[string]any) {
	_ = NewBuilderFromRequest(r).ErrorWithDetails(code, message, category, metadata).Write(w, statusCode)
}

func WriteErrorWithMeta(w http.ResponseWriter, statusCode int, code, message string, meta any) {
	b := NewBuilder().Error(code, message)
	if m, ok := meta.(map[string]any); ok {
		if rid, ok := m["requestId"].(string); ok {
			b.ensureMeta().RequestID = strings.TrimSpace(rid)
		}
		if rt, ok := m["responseTime"].(int64); ok {
			b.ensureMeta().ResponseTime = rt
		}
	}
	_ = b.Write(w, statusCode)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload Envelope) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(payload)
}

func (b *Builder) ensureMeta() *Meta {
	if b.response.Meta == nil {
		b.response.Meta = &Meta{}
	}
	return b.response.Meta
}

func resolveRequestID(r *http.Request) string {
	if r == nil {
		return util.GenerateRequestID()
	}
	if requestID := normalizeRequestID(requestIDFromContext(r.Context())); requestID != "" {
		return requestID
	}
	if requestID := normalizeRequestID(r.Header.Get("X-Request-ID")); requestID != "" {
		return requestID
	}
	return util.GenerateRequestID()
}

func resolveCanonicalRequestID(primary, fallback string) string {
	if requestID := normalizeRequestID(primary); requestID != "" {
		return requestID
	}
	if requestID := normalizeRequestID(fallback); requestID != "" {
		return requestID
	}
	return util.GenerateRequestID()
}

func normalizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(RequestIDContextKey).(string); ok && strings.TrimSpace(requestID) != "" {
		return requestID
	}
	if requestID, ok := ctx.Value(LegacyRequestIDContextKey).(string); ok && strings.TrimSpace(requestID) != "" {
		return requestID
	}
	return ""
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
