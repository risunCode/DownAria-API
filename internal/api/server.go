package api

import (
	"context"
	"encoding/json"
	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	"downaria-api/internal/media"
	"downaria-api/internal/storage"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var startedAt = time.Now()

type ExtractService interface {
	Extract(ctx context.Context, rawURL string, opts extract.ExtractOptions) (*extract.Result, error)
}

type DownloadService interface {
	Download(ctx context.Context, req media.DownloadRequest, update func(storage.JobUpdate)) (*media.DownloadResult, error)
}

type ConvertService interface {
	Convert(ctx context.Context, req media.ConvertRequest) (*media.ConvertResult, error)
}

type MergeService interface {
	Merge(ctx context.Context, req media.MergeRequest, update func(storage.JobUpdate)) (*media.MergeResult, error)
}

type JobService interface {
	Create(jobType string, runner storage.JobRunner) (*storage.Job, error)
	Get(id string) (*storage.Job, error)
	Artifact(id string) (*storage.Job, *storage.Artifact, error)
	Cancel(ctx context.Context, id string) error
	ActiveCount() int
	ActiveJobs() []*storage.Job
}

type ArtifactStore interface {
	SaveFile(srcPath, filename, contentType string, size int64) (*storage.Artifact, error)
}

type URLGuard interface {
	Validate(ctx context.Context, rawURL string) (*url.URL, error)
	ValidateForPlatform(ctx context.Context, rawURL, platform string) (*url.URL, error)
}

func NewRouter(logger *slog.Logger, service ExtractService, downloadSvc DownloadService, convertSvc ConvertService, mergeSvc MergeService, options RouterOptions) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return middleware.Chain(NewMux(logger, service, downloadSvc, convertSvc, mergeSvc, options), middleware.RequestID(), middleware.Logging(logger), middleware.Recover(logger))
}

func NewMux(logger *slog.Logger, service ExtractService, downloadSvc DownloadService, convertSvc ConvertService, mergeSvc MergeService, options RouterOptions) *http.ServeMux {
	_ = logger
	if options.Stats == nil {
		options.Stats = newStatsStore()
	}
	statsStore := options.Stats
	mediaRateLimit := func(handler http.Handler) http.Handler {
		if options.MediaLimiter == nil {
			return handler
		}
		return middleware.RateLimit(options.MediaLimiter, nil)(handler)
	}
	jobRateLimit := func(handler http.Handler) http.Handler {
		if options.JobLimiter == nil {
			return handler
		}
		return middleware.RateLimit(options.JobLimiter, nil)(handler)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /health", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeHealth(w, r, options.Health)
	})))
	mux.Handle("GET /healthz/live", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSuccess(w, r, http.StatusOK, map[string]string{"status": "ok"})
	})))
	mux.Handle("GET /healthz/ready", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if options.ReadyFn != nil && !options.ReadyFn() {
			writeSuccess(w, r, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeSuccess(w, r, http.StatusOK, map[string]string{"status": "ok"})
	})))
	registerAPI := func(prefix string, includeProxy bool) {
		mux.Handle("POST "+prefix+"/extract", mediaRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(service != nil, "extract_service_unavailable", "extract service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			req, err := decodeExtractRequest(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if err := validateURLPolicy(r.Context(), options.Security.Guard, req.URL, ""); err != nil {
				writeError(w, r, err)
				return
			}
			result, err := service.Extract(r.Context(), req.URL, extract.ExtractOptions{CookieHeader: req.Auth.Cookie, UseAuth: req.Auth.Cookie != ""})
			if err != nil {
				writeError(w, r, err)
				return
			}
			statsStore.recordExtraction(1)
			writeSuccess(w, r, http.StatusOK, buildExtractResponse(result))
		})))
		mux.Handle("POST "+prefix+"/download", mediaRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(downloadSvc != nil || mergeSvc != nil || convertSvc != nil, "download_service_unavailable", "download service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			req, err := decodeMediaRequest(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if err := validateMediaRequestURLs(r.Context(), options.Security.Guard, req.URL, req.VideoURL, req.AudioURL, req.Platform); err != nil {
				writeError(w, r, err)
				return
			}
			if shouldRespondAsync(r, req, options) {
				jobReq := JobRequest{Type: "download", MediaRequest: req}
				job, err := createAsyncJob(options, service, downloadSvc, convertSvc, mergeSvc, jobReq)
				if err != nil {
					writeError(w, r, err)
					return
				}
				writeSuccess(w, r, http.StatusAccepted, map[string]any{"mode": "async", "job": buildJobResponse(job, prefix)})
				return
			}

			// If it's a direct media URL and sync request, stream it directly to browser
			if looksLikeDirectMediaURL(req.URL) {
				filename := extract.SanitizeFilename(req.Filename, filepath.Base(strings.TrimSpace(req.URL)))
				streamMedia(w, r, options, req.URL, filename, req.UserAgent, req.Auth.Cookie)
				return
			}

			result, err := handleSmartDownload(r.Context(), req, service, downloadSvc, convertSvc, mergeSvc, nil)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if err := validateOutputSize(result.ContentBytes, options.Security.MaxOutputBytes); err != nil {
				cleanupDownloadOutput(result.FilePath)
				writeError(w, r, err)
				return
			}
			defer cleanupDownloadOutput(result.FilePath)
			if strings.TrimSpace(result.SelectionMode) != "" {
				w.Header().Set("X-DownAria-API-Mode", result.SelectionMode)
			}
			if strings.TrimSpace(result.DownloadMethod) != "" {
				w.Header().Set("X-DownAria-API-Downloader", result.DownloadMethod)
			}
			statsStore.recordDownload(1)
			serveFileDownload(w, r, result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
		})))
		mux.Handle("POST "+prefix+"/convert", mediaRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(convertSvc != nil, "convert_service_unavailable", "convert service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			req, err := decodeMediaRequest(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			result, err := executeConvert(r.Context(), req, convertSvc, options.Security.Guard, options.Security.MaxOutputBytes)
			if err != nil {
				writeError(w, r, err)
				return
			}
			defer os.Remove(result.FilePath)
			statsStore.recordDownload(1)
			serveFileDownload(w, r, result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
		})))
		mux.Handle("POST "+prefix+"/merge", mediaRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(mergeSvc != nil, "merge_service_unavailable", "merge service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			req, err := decodeMediaRequest(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			result, err := executeMerge(r.Context(), req, mergeSvc, options.Security.Guard, options.Security.MaxOutputBytes, nil)
			if err != nil {
				writeError(w, r, err)
				return
			}
			defer os.Remove(result.FilePath)
			statsStore.recordDownload(1)
			serveFileDownload(w, r, result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
		})))
		mux.Handle("POST "+prefix+"/jobs", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(options.Jobs != nil && options.ArtifactStore != nil, "job_service_unavailable", "job service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			jobReq, err := decodeJobRequest(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			job, err := createAsyncJob(options, service, downloadSvc, convertSvc, mergeSvc, jobReq)
			if err != nil {
				writeError(w, r, err)
				return
			}
			writeSuccess(w, r, http.StatusAccepted, buildJobResponse(job, prefix))
		})))
		mux.Handle("GET "+prefix+"/jobs/{id}", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(options.Jobs != nil, "job_service_unavailable", "job service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			job, err := options.Jobs.Get(r.PathValue("id"))
			if err != nil {
				writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "job_not_found", "job not found", false, err))
				return
			}
			writeSuccess(w, r, http.StatusOK, buildJobResponse(job, prefix))
		})))
		mux.Handle("DELETE "+prefix+"/jobs/{id}", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(options.Jobs != nil, "job_service_unavailable", "job service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			id := r.PathValue("id")
			if err := options.Jobs.Cancel(r.Context(), id); err != nil {
				writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "job_not_found", "job not found", false, err))
				return
			}
			writeSuccess(w, r, http.StatusOK, map[string]string{"id": id, "state": storage.StateCancelled})
		})))
		mux.Handle("GET "+prefix+"/jobs/{id}/artifact", jobRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := requireService(options.Jobs != nil, "job_service_unavailable", "job service is not configured"); err != nil {
				writeError(w, r, err)
				return
			}
			_, artifact, err := options.Jobs.Artifact(r.PathValue("id"))
			if err != nil || artifact == nil {
				writeError(w, r, extract.WrapCode(extract.KindInvalidInput, "artifact_missing", "artifact is unavailable", false, err))
				return
			}
			statsStore.recordDownload(1)
			serveFileDownload(w, r, artifact.Path, artifact.Filename, artifact.ContentType, artifact.ContentBytes)
		})))
		if includeProxy {
			mux.Handle("GET "+prefix+"/proxy", mediaRateLimit(handleProxy(options)))
		}
		mux.Handle("GET "+prefix+"/stats", jobRateLimit(handleStats(statsStore)))
		mux.Handle("POST "+prefix+"/stats/log", jobRateLimit(handleStatsLog(statsStore)))
	}
	registerAPI("/api/v1", true)
	return mux
}

type successEnvelope struct {
	Success        bool  `json:"success"`
	ResponseTimeMS int64 `json:"response_time_ms"`
	Data           any   `json:"data"`
}

type errorEnvelope struct {
	Success        bool  `json:"success"`
	ResponseTimeMS int64 `json:"response_time_ms"`
	Error          struct {
		Kind      string `json:"kind,omitempty"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable,omitempty"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}

type extractResponseData struct {
	URL            string             `json:"url"`
	Platform       string             `json:"platform"`
	ExtractProfile string             `json:"extract_profile,omitempty"`
	ContentType    string             `json:"content_type"`
	Title          string             `json:"title,omitempty"`
	Author         extract.Author     `json:"author,omitempty"`
	Engagement     extract.Engagement `json:"engagement,omitempty"`
	Filename       string             `json:"filename,omitempty"`
	ThumbnailURL   string             `json:"thumbnail_url,omitempty"`
	Visibility     string             `json:"visibility,omitempty"`
	FileSizeBytes  int64              `json:"file_size_bytes,omitempty"`
	Media          []mediaItemResp    `json:"media"`
}

type mediaItemResp struct {
	Index         int          `json:"index"`
	Type          string       `json:"type"`
	Filename      string       `json:"filename,omitempty"`
	ThumbnailURL  string       `json:"thumbnail_url,omitempty"`
	FileSizeBytes int64        `json:"file_size_bytes,omitempty"`
	Sources       []sourceResp `json:"sources"`
}

type sourceResp struct {
	Quality       string                `json:"quality,omitempty"`
	URL           string                `json:"url"`
	MIMEType      string                `json:"mime_type,omitempty"`
	FileSizeBytes int64                 `json:"file_size_bytes,omitempty"`
	StreamProfile extract.StreamProfile `json:"stream_profile,omitempty"`
	IsProgressive bool                  `json:"is_progressive"`
	NeedsMerge    bool                  `json:"needs_merge"`
}

type jobResponseData struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	State           string            `json:"state"`
	Message         string            `json:"message,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	SelectedFormats []string          `json:"selected_formats,omitempty"`
	Artifact        *storage.Artifact `json:"artifact,omitempty"`
	Error           *storage.JobError `json:"error,omitempty"`
	StatusURL       string            `json:"status_url"`
	ArtifactURL     string            `json:"artifact_url"`
}

func writeSuccess(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, successEnvelope{Success: true, ResponseTimeMS: responseTimeMS(r), Data: data})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var payload errorEnvelope
	payload.Success = false
	payload.ResponseTimeMS = responseTimeMS(r)
	if appErr := extract.AsAppError(err); appErr != nil && appErr.Kind != "" {
		payload.Error.Kind = string(appErr.Kind)
		payload.Error.Code = appErr.CodeValue()
		payload.Error.Retryable = appErr.Retryable
	} else {
		payload.Error.Kind = string(extract.KindInternal)
		payload.Error.Code = string(extract.KindInternal)
	}
	payload.Error.Message = extract.SafeMessage(err)
	if r != nil {
		payload.Error.RequestID = middleware.FromContext(r.Context())
	}
	writeJSON(w, statusFromError(err), payload)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func responseTimeMS(r *http.Request) int64 {
	if r == nil {
		return 0
	}
	startedAt := middleware.StartedAtFromContext(r.Context())
	if startedAt.IsZero() {
		return 0
	}
	return time.Since(startedAt).Milliseconds()
}

func buildExtractResponse(result *extract.Result) extractResponseData {
	if result == nil {
		return extractResponseData{}
	}
	out := extractResponseData{
		URL:            strings.TrimSpace(result.SourceURL),
		Platform:       strings.TrimSpace(result.Platform),
		ExtractProfile: strings.TrimSpace(result.ExtractProfile),
		ContentType:    strings.TrimSpace(result.ContentType),
		Title:          strings.TrimSpace(result.Title),
		Author:         result.Author,
		Engagement:     result.Engagement,
		Filename:       strings.TrimSpace(result.Filename),
		ThumbnailURL:   primaryThumbnailURL(result),
		Visibility:     strings.TrimSpace(result.Visibility),
		FileSizeBytes:  result.FileSizeBytes,
	}
	out.Media = filterAndMapMedia(result.Media)
	return out
}

func primaryThumbnailURL(result *extract.Result) string {
	if result == nil {
		return ""
	}
	for _, item := range result.Media {
		if strings.TrimSpace(item.ThumbnailURL) != "" {
			return strings.TrimSpace(item.ThumbnailURL)
		}
	}
	return ""
}

func filterAndMapMedia(items []extract.MediaItem) []mediaItemResp {
	filtered := make([]mediaItemResp, 0, len(items))
	for _, item := range items {
		sources := make([]sourceResp, 0, len(item.Sources))
		itemTotal := int64(0)
		for _, source := range item.Sources {
			if source.FileSizeBytes < 0 {
				continue
			}
			sources = append(sources, sourceResp{
				Quality:       source.Quality,
				URL:           source.URL,
				MIMEType:      source.MIMEType,
				FileSizeBytes: source.FileSizeBytes,
				StreamProfile: source.StreamProfile,
				IsProgressive: source.IsProgressive,
				NeedsMerge:    (source.HasVideo && !source.HasAudio) || source.StreamProfile == extract.StreamProfileVideoOnlyAdaptive,
			})
			itemTotal += source.FileSizeBytes
		}
		if len(sources) == 0 {
			continue
		}
		filtered = append(filtered, mediaItemResp{
			Index:         item.Index,
			Type:          item.Type,
			Filename:      item.Filename,
			ThumbnailURL:  item.ThumbnailURL,
			FileSizeBytes: itemTotal,
			Sources:       sources,
		})
	}
	return filtered
}
