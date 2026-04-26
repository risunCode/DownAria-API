package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"downaria-api/internal/extract"
	"downaria-api/internal/netutil"
	runtime "downaria-api/internal/runtime"
)

type statsEvent string

const (
	statsEventVisit      statsEvent = "visit"
	statsEventExtraction statsEvent = "extraction"
	statsEventDownload   statsEvent = "download"
)

type statsStore struct {
	mu              sync.Mutex
	currentDate     string
	updatedAt       time.Time
	lastCleanup     time.Time
	visitorsTotal   int64
	visitorsToday   int64
	extractTotal    int64
	extractToday    int64
	downloadTotal   int64
	downloadToday   int64
	seenVisitorKeys map[string]struct{}
	filePath        string
}

type statsResponse struct {
	Visitors struct {
		Today int64 `json:"today"`
		Total int64 `json:"total"`
	} `json:"visitors"`
	Extractions struct {
		Today int64 `json:"today"`
		Total int64 `json:"total"`
	} `json:"extractions"`
	Downloads struct {
		Today int64 `json:"today"`
		Total int64 `json:"total"`
	} `json:"downloads"`
	UpdatedAt string `json:"updated_at"`
}

type statsLogRequest struct {
	Event  string `json:"event"`
	Amount int64  `json:"amount"`
}

var _ = newStatsStore // keep newStatsStore reachable for default init in NewMux

func newStatsStore() *statsStore {
	root, err := runtime.EnsureStatsDir()
	filePath := ""
	if err == nil {
		filePath = filepath.Join(root, "stats.json")
	}
	store := &statsStore{
		currentDate:     time.Now().Format("2006-01-02"),
		seenVisitorKeys: make(map[string]struct{}),
		filePath:        filePath,
	}
	store.loadFromFile()
	return store
}

func (s *statsStore) ensureDateLocked(now time.Time) {
	date := now.Format("2006-01-02")
	if s.currentDate == date {
		return
	}
	s.currentDate = date
	s.visitorsToday = 0
	s.extractToday = 0
	s.downloadToday = 0
	s.seenVisitorKeys = make(map[string]struct{})
}

// cleanupStaleVisitors clears seenVisitorKeys if 24 hours have passed since last cleanup.
// Must be called with s.mu held.
func (s *statsStore) cleanupStaleVisitors(now time.Time) {
	if now.Sub(s.lastCleanup) >= 24*time.Hour {
		s.seenVisitorKeys = make(map[string]struct{})
		s.lastCleanup = now
	}
}

func (s *statsStore) recordVisit(visitorKey string) {
	now := time.Now()
	key := strings.TrimSpace(visitorKey)
	if key == "" {
		key = "anonymous"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureDateLocked(now)
	s.cleanupStaleVisitors(now)
	if _, exists := s.seenVisitorKeys[key]; exists {
		return
	}
	s.seenVisitorKeys[key] = struct{}{}
	s.updatedAt = now
	s.visitorsToday++
	s.visitorsTotal++
	s.persistLocked()
}

func (s *statsStore) recordExtraction(amount int64) {
	if amount <= 0 {
		amount = 1
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureDateLocked(now)
	s.updatedAt = now
	s.extractToday += amount
	s.extractTotal += amount
	s.persistLocked()
}

func (s *statsStore) recordDownload(amount int64) {
	if amount <= 0 {
		amount = 1
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureDateLocked(now)
	s.updatedAt = now
	s.downloadToday += amount
	s.downloadTotal += amount
	s.persistLocked()
}

func (s *statsStore) snapshot() statsResponse {
	now := time.Now()
	s.mu.Lock()
	s.ensureDateLocked(now)
	if s.updatedAt.IsZero() {
		s.updatedAt = now
	}
	resp := statsResponse{
		UpdatedAt: s.updatedAt.Format(time.RFC3339),
	}
	resp.Visitors.Today = s.visitorsToday
	resp.Visitors.Total = s.visitorsTotal
	resp.Extractions.Today = s.extractToday
	resp.Extractions.Total = s.extractTotal
	resp.Downloads.Today = s.downloadToday
	resp.Downloads.Total = s.downloadTotal
	s.mu.Unlock()
	return resp
}

func (s *statsStore) loadFromFile() {
	if s.filePath == "" {
		return
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var stored struct {
		CurrentDate     string   `json:"current_date"`
		UpdatedAt       string   `json:"updated_at"`
		VisitorsTotal   int64    `json:"visitors_total"`
		VisitorsToday   int64    `json:"visitors_today"`
		ExtractTotal    int64    `json:"extract_total"`
		ExtractToday    int64    `json:"extract_today"`
		DownloadTotal   int64    `json:"download_total"`
		DownloadToday   int64    `json:"download_today"`
		SeenVisitorKeys []string `json:"seen_visitor_keys"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	s.currentDate = stored.CurrentDate
	if parsed, err := time.Parse(time.RFC3339, stored.UpdatedAt); err == nil {
		s.updatedAt = parsed
	}
	s.visitorsTotal = stored.VisitorsTotal
	s.visitorsToday = stored.VisitorsToday
	s.extractTotal = stored.ExtractTotal
	s.extractToday = stored.ExtractToday
	s.downloadTotal = stored.DownloadTotal
	s.downloadToday = stored.DownloadToday
	s.seenVisitorKeys = make(map[string]struct{})
	for _, key := range stored.SeenVisitorKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		s.seenVisitorKeys[trimmed] = struct{}{}
	}
}

func (s *statsStore) persistLocked() {
	if s.filePath == "" {
		return
	}
	payload := struct {
		CurrentDate     string   `json:"current_date"`
		UpdatedAt       string   `json:"updated_at"`
		VisitorsTotal   int64    `json:"visitors_total"`
		VisitorsToday   int64    `json:"visitors_today"`
		ExtractTotal    int64    `json:"extract_total"`
		ExtractToday    int64    `json:"extract_today"`
		DownloadTotal   int64    `json:"download_total"`
		DownloadToday   int64    `json:"download_today"`
		SeenVisitorKeys []string `json:"seen_visitor_keys"`
	}{
		CurrentDate:     s.currentDate,
		UpdatedAt:       s.updatedAt.Format(time.RFC3339),
		VisitorsTotal:   s.visitorsTotal,
		VisitorsToday:   s.visitorsToday,
		ExtractTotal:    s.extractTotal,
		ExtractToday:    s.extractToday,
		DownloadTotal:   s.downloadTotal,
		DownloadToday:   s.downloadToday,
		SeenVisitorKeys: mapKeys(s.seenVisitorKeys),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return
	}
	_ = os.Remove(s.filePath)
	_ = os.Rename(tmpPath, s.filePath)
}

func mapKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func handleStats(statsStore *statsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeSuccess(w, r, http.StatusOK, statsStore.snapshot())
	}
}

func handleStatsLog(statsStore *statsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body statsLogRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "invalid_request_body", "invalid request body", false, err))
			return
		}
		switch statsEvent(strings.TrimSpace(body.Event)) {
		case statsEventVisit:
			statsStore.recordVisit(netutil.ClientIP(r))
		case statsEventExtraction:
			statsStore.recordExtraction(body.Amount)
		case statsEventDownload:
			statsStore.recordDownload(body.Amount)
		default:
			writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "unsupported_stats_event", "unsupported stats event", false, nil))
			return
		}
		writeSuccess(w, r, http.StatusOK, map[string]any{"ok": true})
	}
}
