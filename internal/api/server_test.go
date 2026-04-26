package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	"downaria-api/internal/storage"
)

type stubExtractService struct{ result *extract.Result }

func (s stubExtractService) Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error) {
	return s.result, nil
}

func TestExtractRouteSuccessEnvelope(t *testing.T) {
	for _, path := range []string{"/api/v1/extract"} {
		t.Run(path, func(t *testing.T) {
			svc := stubExtractService{result: &extract.Result{SourceURL: "https://example.com/post", Platform: "x", ContentType: "video", Media: []extract.MediaItem{{Type: "video", Sources: []extract.MediaSource{{URL: "https://cdn.example.com/a.mp4", FileSizeBytes: 1, HasVideo: true, HasAudio: true, IsProgressive: true}}}}}}
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"url":"https://example.com/post"}`))
			w := httptest.NewRecorder()
			NewMux(nil, svc, nil, nil, nil, RouterOptions{}).ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			var payload map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["success"] != true || payload["data"] == nil {
				t.Fatalf("bad envelope: %#v", payload)
			}
		})
	}
}

func TestExtractRouteMissingServiceErrorEnvelope(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/extract", strings.NewReader(`{"url":"https://example.com/post"}`))
	w := httptest.NewRecorder()
	NewMux(nil, nil, nil, nil, nil, RouterOptions{}).ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Success || payload.Error.Code != "extract_service_unavailable" {
		t.Fatalf("bad error payload: %#v", payload)
	}
}

func TestCancelJobRouteCancelsPendingJob(t *testing.T) {
	artifacts, err := storage.NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := storage.NewJobManager(filepath.Join(t.TempDir(), "jobs"), artifacts, time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()

	started := make(chan struct{})
	job, err := jobs.Create("download", func(ctx context.Context, update func(storage.JobUpdate)) (*storage.Artifact, storage.JobMetadata, *storage.JobError) {
		close(started)
		<-ctx.Done()
		return nil, storage.JobMetadata{}, &storage.JobError{Code: "cancelled", Message: ctx.Err().Error()}
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	r := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+job.ID, nil)
	w := httptest.NewRecorder()
	NewMux(nil, nil, nil, nil, nil, RouterOptions{Jobs: jobs}).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	got := waitAPIJobState(t, jobs, job.ID, storage.StateCancelled)
	if got.State != storage.StateCancelled {
		t.Fatalf("state = %s", got.State)
	}
}

func TestWebJobRouteReturnsWebURLs(t *testing.T) {
	artifacts, err := storage.NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := storage.NewJobManager(filepath.Join(t.TempDir(), "jobs"), artifacts, time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer jobs.Close()
	job, err := jobs.Create("download", func(ctx context.Context, update func(storage.JobUpdate)) (*storage.Artifact, storage.JobMetadata, *storage.JobError) {
		return nil, storage.JobMetadata{}, &storage.JobError{Code: "boom", Message: "failed"}
	})
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	w := httptest.NewRecorder()
	NewMux(nil, nil, nil, nil, nil, RouterOptions{Jobs: jobs}).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var payload struct {
		Data struct {
			StatusURL   string `json:"status_url"`
			ArtifactURL string `json:"artifact_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.StatusURL != "/api/v1/jobs/"+job.ID || payload.Data.ArtifactURL != "/api/v1/jobs/"+job.ID+"/artifact" {
		t.Fatalf("bad urls: %#v", payload.Data)
	}
}

func waitAPIJobState(t *testing.T, jobs *storage.JobManager, id, state string) *storage.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := jobs.Get(id)
		if err == nil && job.State == state {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, state)
	return nil
}

func TestHealthzEndpoints(t *testing.T) {
	mux := NewMux(nil, nil, nil, nil, nil, RouterOptions{ReadyFn: func() bool { return false }})

	liveReq := httptest.NewRequest(http.MethodGet, "/healthz/live", nil)
	liveRec := httptest.NewRecorder()
	mux.ServeHTTP(liveRec, liveReq)
	if liveRec.Code != http.StatusOK {
		t.Fatalf("live status = %d", liveRec.Code)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	readyRec := httptest.NewRecorder()
	mux.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d", readyRec.Code)
	}

	mux = NewMux(nil, nil, nil, nil, nil, RouterOptions{ReadyFn: func() bool { return true }})
	readyRec = httptest.NewRecorder()
	mux.ServeHTTP(readyRec, readyReq)
	if readyRec.Code != http.StatusOK {
		t.Fatalf("ready status after ready = %d", readyRec.Code)
	}

	htmlReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	htmlReq.Header.Set("Accept", "text/html")
	htmlRec := httptest.NewRecorder()
	mux.ServeHTTP(htmlRec, htmlReq)
	if htmlRec.Code != http.StatusOK {
		t.Fatalf("html status = %d", htmlRec.Code)
	}
	if !strings.Contains(htmlRec.Body.String(), "<!DOCTYPE html>") {
		t.Fatalf("expected html response, got: %s", htmlRec.Body.String())
	}

	jsonReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	jsonRec := httptest.NewRecorder()
	mux.ServeHTTP(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("json health status = %d", jsonRec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json response, got: %s", jsonRec.Body.String())
	}
	if payload["success"] != true {
		t.Fatalf("bad json response: %v", payload)
	}
}

func TestMediaRoutesAreRateLimited(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, 1)
	mux := NewMux(nil, nil, nil, nil, nil, RouterOptions{MediaLimiter: limiter})

	for i, want := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/download", strings.NewReader(`{"url":"https://example.com/a.mp4"}`))
		r.RemoteAddr = "203.0.113.10:1234"
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, r)

		if w.Code != want {
			t.Fatalf("request %d status = %d, want %d", i+1, w.Code, want)
		}
	}
}

func TestAllRoutesAreRateLimitedByCategory(t *testing.T) {
	mediaLimiter := middleware.NewRateLimiter(1, 1)
	jobLimiter := middleware.NewRateLimiter(1, 1)
	mux := NewMux(nil, nil, nil, nil, nil, RouterOptions{
		MediaLimiter: mediaLimiter,
		JobLimiter:   jobLimiter,
	})
	addrCounter := 10

	checkLimited := func(method, path, body string, wantFirst int) {
		t.Helper()
		addrCounter++
		remoteAddr := "203.0.113." + strconv.Itoa(addrCounter) + ":1234"
		for i, want := range []int{wantFirst, http.StatusTooManyRequests} {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.RemoteAddr = remoteAddr
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%s %s request %d status = %d, want %d", method, path, i+1, w.Code, want)
			}
		}
	}

	checkLimited(http.MethodPost, "/api/v1/extract", `{"url":"https://example.com/post"}`, http.StatusInternalServerError)
	checkLimited(http.MethodGet, "/api/v1/proxy?url=https%3A%2F%2Fexample.com%2Fa.mp4", "", http.StatusInternalServerError)
	checkLimited(http.MethodPost, "/api/v1/download", `{"url":"https://example.com/a.mp4"}`, http.StatusInternalServerError)
	checkLimited(http.MethodPost, "/api/v1/convert", `{"url":"https://example.com/a.mp4"}`, http.StatusInternalServerError)
	checkLimited(http.MethodPost, "/api/v1/merge", `{"url":"https://example.com/a.mp4","video_url":"https://example.com/v.mp4","audio_url":"https://example.com/a.m4a"}`, http.StatusInternalServerError)
	checkLimited(http.MethodGet, "/health", "", http.StatusOK)
	checkLimited(http.MethodGet, "/healthz/live", "", http.StatusOK)
	checkLimited(http.MethodGet, "/healthz/ready", "", http.StatusOK)
	checkLimited(http.MethodPost, "/api/v1/jobs", `{"type":"download","url":"https://example.com/a.mp4"}`, http.StatusInternalServerError)
	checkLimited(http.MethodGet, "/api/v1/jobs/job_123", "", http.StatusInternalServerError)
	checkLimited(http.MethodDelete, "/api/v1/jobs/job_123", "", http.StatusInternalServerError)
	checkLimited(http.MethodGet, "/api/v1/jobs/job_123/artifact", "", http.StatusInternalServerError)
	checkLimited(http.MethodGet, "/api/v1/stats", "", http.StatusOK)
	checkLimited(http.MethodPost, "/api/v1/stats/log", `{"kind":"test"}`, http.StatusBadRequest)
}
