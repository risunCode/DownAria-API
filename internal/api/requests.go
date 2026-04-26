package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"downaria-api/internal/extract"
	"downaria-api/internal/media"
	"downaria-api/internal/storage"
)

type ExtractRequest struct {
	URL  string `json:"url"`
	Auth struct {
		Cookie string `json:"cookie,omitempty"`
	} `json:"auth,omitempty"`
}

type MediaRequest struct {
	URL       string `json:"url"`
	Filename  string `json:"filename,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Quality   string `json:"quality,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Format    string `json:"format,omitempty"`
	AudioOnly bool   `json:"audio_only,omitempty"`
	Async     bool   `json:"async,omitempty"`
	VideoURL  string `json:"video_url,omitempty"`
	AudioURL  string `json:"audio_url,omitempty"`
	Auth      struct {
		Cookie string `json:"cookie,omitempty"`
	} `json:"auth,omitempty"`
}

type JobRequest struct {
	Type string `json:"type"`
	MediaRequest
}

type smartDownloadResult struct {
	FilePath       string
	Filename       string
	ContentType    string
	ContentBytes   int64
	SelectionMode  string
	DownloadMethod string
}

func decodeJSONBody[T any](r *http.Request) (T, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var v T
	if err := decoder.Decode(&v); err != nil {
		var zero T
		return zero, extract.WrapCode(extract.KindInvalidInput, "invalid_request_body", "invalid request body", false, err)
	}
	return v, nil
}

func normalizeMediaFields(req *MediaRequest) {
	req.URL = strings.TrimSpace(req.URL)
	req.Filename = strings.TrimSpace(req.Filename)
	req.Platform = strings.TrimSpace(req.Platform)
	req.Quality = strings.TrimSpace(req.Quality)
	req.UserAgent = strings.TrimSpace(req.UserAgent)
	req.Format = strings.TrimSpace(req.Format)
	req.VideoURL = strings.TrimSpace(req.VideoURL)
	req.AudioURL = strings.TrimSpace(req.AudioURL)
	req.Auth.Cookie = strings.TrimSpace(req.Auth.Cookie)
}

func validateCookieSize(cookie string) error {
	if len(cookie) > 16384 {
		return extract.WrapCode(extract.KindInvalidInput, "cookie_too_large", "cookie is too large", false, nil)
	}
	return nil
}

func decodeExtractRequest(r *http.Request) (ExtractRequest, error) {
	req, err := decodeJSONBody[ExtractRequest](r)
	if err != nil {
		return ExtractRequest{}, err
	}
	req.URL = strings.TrimSpace(req.URL)
	req.Auth.Cookie = strings.TrimSpace(req.Auth.Cookie)
	if req.URL == "" {
		return ExtractRequest{}, extract.WrapCode(extract.KindInvalidInput, "extract_url_required", "url is required", false, nil)
	}
	if err := validateCookieSize(req.Auth.Cookie); err != nil {
		return ExtractRequest{}, err
	}
	return req, nil
}

func decodeMediaRequest(r *http.Request) (MediaRequest, error) {
	req, err := decodeJSONBody[MediaRequest](r)
	if err != nil {
		return MediaRequest{}, err
	}
	normalizeMediaFields(&req)
	if err := validateMediaRequestShape(req.URL, req.VideoURL, req.AudioURL); err != nil {
		return MediaRequest{}, err
	}
	if err := validateCookieSize(req.Auth.Cookie); err != nil {
		return MediaRequest{}, err
	}
	return req, nil
}

func decodeJobRequest(r *http.Request) (JobRequest, error) {
	req, err := decodeJSONBody[JobRequest](r)
	if err != nil {
		return JobRequest{}, err
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	normalizeMediaFields(&req.MediaRequest)
	if req.Type != "download" && req.Type != "convert" && req.Type != "merge" {
		return JobRequest{}, extract.WrapCode(extract.KindInvalidInput, "job_type_invalid", "job type is invalid", false, nil)
	}
	if err := validateJobRequestShape(req); err != nil {
		return JobRequest{}, err
	}
	if err := validateCookieSize(req.Auth.Cookie); err != nil {
		return JobRequest{}, err
	}
	return req, nil
}

func shouldRespondAsync(r *http.Request, req MediaRequest, options RouterOptions) bool {
	if options.Jobs == nil || options.ArtifactStore == nil {
		return false
	}
	if req.Async {
		return true
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Prefer"))), "respond-async") {
		return true
	}
	if req.AudioOnly && !looksLikeDirectMediaURL(req.URL) {
		return true
	}
	if strings.TrimSpace(req.Quality) != "" {
		return true
	}
	if strings.TrimSpace(req.VideoURL) != "" || strings.TrimSpace(req.AudioURL) != "" {
		return true
	}
	return false
}

func validateURLPolicy(ctx context.Context, guard URLGuard, rawURL, platform string) error {
	if guard == nil || strings.TrimSpace(rawURL) == "" {
		return nil
	}
	var err error
	if strings.TrimSpace(platform) != "" {
		_, err = guard.ValidateForPlatform(ctx, rawURL, platform)
	} else {
		_, err = guard.Validate(ctx, rawURL)
	}
	if err != nil {
		code := "blocked_destination"
		if isMirrorWorkerBlockError(err) {
			code = "blocked_mirror_worker"
		}
		return extract.WrapCode(extract.KindInvalidInput, code, "url is blocked by security policy", false, err)
	}
	return nil
}

func isMirrorWorkerBlockError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "mirror worker")
}

func validateMediaRequestURLs(ctx context.Context, guard URLGuard, rawURL, videoURL, audioURL, platform string) error {
	if strings.TrimSpace(rawURL) != "" {
		return validateURLPolicy(ctx, guard, rawURL, platform)
	}
	if err := validateURLPolicy(ctx, guard, videoURL, platform); err != nil {
		return err
	}
	if err := validateURLPolicy(ctx, guard, audioURL, platform); err != nil {
		return err
	}
	return nil
}

func validateMediaRequestShape(rawURL, videoURL, audioURL string) error {
	hasURL := strings.TrimSpace(rawURL) != ""
	hasVideo := strings.TrimSpace(videoURL) != ""
	hasAudio := strings.TrimSpace(audioURL) != ""
	if hasURL && (hasVideo || hasAudio) {
		return extract.WrapCode(extract.KindInvalidInput, "media_request_ambiguous", "use either url or explicit video_url/audio_url, not both", false, nil)
	}
	if !hasURL && (!hasVideo || !hasAudio) {
		return extract.WrapCode(extract.KindInvalidInput, "media_url_required", "url is required", false, nil)
	}
	return nil
}

func validateJobRequestShape(req JobRequest) error {
	if err := validateMediaRequestShape(req.URL, req.VideoURL, req.AudioURL); err != nil {
		if req.Type == "convert" && strings.TrimSpace(req.URL) == "" {
			return extract.WrapCode(extract.KindInvalidInput, "convert_url_required", "url is required", false, nil)
		}
		if req.Type == "merge" && strings.TrimSpace(req.URL) == "" {
			return extract.WrapCode(extract.KindInvalidInput, "merge_pair_required", "video_url and audio_url are required", false, nil)
		}
		return err
	}
	if req.Type == "convert" && strings.TrimSpace(req.URL) == "" {
		return extract.WrapCode(extract.KindInvalidInput, "convert_url_required", "url is required", false, nil)
	}
	return nil
}

func validateOutputSize(size, maxBytes int64) error {
	if maxBytes <= 0 || size <= maxBytes {
		return nil
	}
	return extract.WrapCode(extract.KindInvalidInput, "output_too_large", "output exceeds configured size limit", false, nil)
}

func selectedFormatsForDownloadJob(req JobRequest, mode string) []string {
	parts := []string{}
	if strings.TrimSpace(req.Quality) != "" {
		parts = append(parts, strings.TrimSpace(req.Quality))
	}
	if strings.TrimSpace(req.Format) != "" {
		parts = append(parts, strings.TrimSpace(req.Format))
	}
	if strings.TrimSpace(mode) != "" {
		parts = append(parts, strings.TrimSpace(mode))
	}
	return parts
}

func executeConvert(ctx context.Context, req MediaRequest, convertSvc ConvertService, guard URLGuard, maxOutputBytes int64) (*media.ConvertResult, error) {
	if err := validateURLPolicy(ctx, guard, req.URL, req.Platform); err != nil {
		return nil, err
	}
	result, err := convertSvc.Convert(ctx, media.ConvertRequest{URL: req.URL, Filename: req.Filename, Format: req.Format, AudioOnly: req.AudioOnly, UserAgent: req.UserAgent, CookieHeader: req.Auth.Cookie})
	if err != nil {
		return nil, err
	}
	if err := validateOutputSize(result.ContentBytes, maxOutputBytes); err != nil {
		_ = os.Remove(result.FilePath)
		return nil, err
	}
	return result, nil
}

func executeMerge(ctx context.Context, req MediaRequest, mergeSvc MergeService, guard URLGuard, maxOutputBytes int64, update func(storage.JobUpdate)) (*media.MergeResult, error) {
	if err := validateMediaRequestURLs(ctx, guard, req.URL, req.VideoURL, req.AudioURL, req.Platform); err != nil {
		return nil, err
	}
	result, err := mergeSvc.Merge(ctx, media.MergeRequest{URL: req.URL, Quality: req.Quality, VideoURL: req.VideoURL, AudioURL: req.AudioURL, Filename: req.Filename, Format: req.Format, UserAgent: req.UserAgent, CookieHeader: req.Auth.Cookie}, update)
	if err != nil {
		return nil, err
	}
	if err := validateOutputSize(result.ContentBytes, maxOutputBytes); err != nil {
		_ = os.Remove(result.FilePath)
		return nil, err
	}
	return result, nil
}

func buildJobResponse(job *storage.Job, prefix string) jobResponseData {
	var out jobResponseData
	if job == nil {
		return out
	}
	prefix = normalizeRoutePrefix(prefix)
	out.ID = job.ID
	out.Type = job.Type
	out.State = job.State
	out.Message = job.Message
	out.CreatedAt = job.CreatedAt
	out.UpdatedAt = job.UpdatedAt
	out.SelectedFormats = job.SelectedFormats
	out.Artifact = job.Artifact
	out.Error = job.Error
	out.StatusURL = prefix + "/jobs/" + job.ID
	out.ArtifactURL = prefix + "/jobs/" + job.ID + "/artifact"
	return out
}

func normalizeRoutePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/api/v1"
	}
	prefix = strings.TrimRight(prefix, "/")
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return prefix
}

func createAsyncJob(options RouterOptions, extractSvc ExtractService, downloadSvc DownloadService, convertSvc ConvertService, mergeSvc MergeService, req JobRequest) (*storage.Job, error) {
	if options.Jobs == nil || options.ArtifactStore == nil {
		return nil, extract.WrapCode(extract.KindInternal, "job_service_unavailable", "job service is not configured", false, nil)
	}
	runner := func(ctx context.Context, update func(storage.JobUpdate)) (*storage.Artifact, storage.JobMetadata, *storage.JobError) {
		switch req.Type {
		case "download":
			if downloadSvc == nil && mergeSvc == nil && convertSvc == nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "download_service_unavailable", Message: "download service is not configured"}
			}
			if err := validateMediaRequestURLs(ctx, options.Security.Guard, req.URL, req.VideoURL, req.AudioURL, req.Platform); err != nil {
				return nil, storage.JobMetadata{}, jobError(err)
			}
			mediaReq := MediaRequest{URL: req.URL, Filename: req.Filename, Platform: req.Platform, Quality: req.Quality, UserAgent: req.UserAgent, Format: req.Format, VideoURL: req.VideoURL, AudioURL: req.AudioURL, AudioOnly: req.AudioOnly}
			mediaReq.Auth.Cookie = req.Auth.Cookie
			result, err := handleSmartDownload(ctx, mediaReq, extractSvc, downloadSvc, convertSvc, mergeSvc, update)
			if err != nil {
				return nil, storage.JobMetadata{}, jobError(err)
			}
			if err := validateOutputSize(result.ContentBytes, options.Security.MaxOutputBytes); err != nil {
				cleanupDownloadOutput(result.FilePath)
				return nil, storage.JobMetadata{}, jobError(err)
			}
			artifact, err := options.ArtifactStore.SaveFile(result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
			if err != nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "artifact_store_failed", Message: err.Error()}
			}
			return artifact, storage.JobMetadata{SelectedFormats: selectedFormatsForDownloadJob(req, result.SelectionMode)}, nil
		case "convert":
			if convertSvc == nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "convert_service_unavailable", Message: "convert service is not configured"}
			}
			result, err := executeConvert(ctx, req.MediaRequest, convertSvc, options.Security.Guard, options.Security.MaxOutputBytes)
			if err != nil {
				return nil, storage.JobMetadata{}, jobError(err)
			}
			artifact, err := options.ArtifactStore.SaveFile(result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
			if err != nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "artifact_store_failed", Message: err.Error()}
			}
			return artifact, storage.JobMetadata{}, nil
		case "merge":
			if mergeSvc == nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "merge_service_unavailable", Message: "merge service is not configured"}
			}
			result, err := executeMerge(ctx, req.MediaRequest, mergeSvc, options.Security.Guard, options.Security.MaxOutputBytes, update)
			if err != nil {
				return nil, storage.JobMetadata{}, jobError(err)
			}
			artifact, err := options.ArtifactStore.SaveFile(result.FilePath, result.Filename, result.ContentType, result.ContentBytes)
			if err != nil {
				return nil, storage.JobMetadata{}, &storage.JobError{Code: "artifact_store_failed", Message: err.Error()}
			}
			return artifact, storage.JobMetadata{SelectedFormats: result.SelectedFormatIDs}, nil
		default:
			return nil, storage.JobMetadata{}, &storage.JobError{Code: "job_type_invalid", Message: "job type is invalid"}
		}
	}
	job, err := options.Jobs.Create(req.Type, runner)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too many concurrent jobs") {
			return nil, extract.WrapCode(extract.KindRateLimited, "job_over_capacity", "too many concurrent jobs", true, err)
		}
		return nil, err
	}
	return job, nil
}

func jobError(err error) *storage.JobError {
	if appErr := extract.AsAppError(err); appErr != nil {
		return &storage.JobError{Code: appErr.CodeValue(), Message: extract.SafeMessage(err), Retryable: appErr.Retryable}
	}
	if err == nil {
		return nil
	}
	return &storage.JobError{Code: "internal", Message: extract.SafeMessage(err)}
}

func requireService(available bool, code, message string) error {
	if available {
		return nil
	}
	return extract.WrapCode(extract.KindInternal, code, message, false, nil)
}
