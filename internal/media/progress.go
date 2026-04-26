package media

import (
	"context"

	"downaria-api/internal/extract"
	"downaria-api/internal/netutil"
	"downaria-api/internal/storage"
	"os"
	"path/filepath"
	"strings"
)

func (s *Merger) downloadCandidate(ctx context.Context, pageURL, outputBase string, candidate *Candidate, req MergeRequest, update func(storage.JobUpdate)) (*downloadedArtifact, error) {
	if candidate == nil {
		return nil, extract.WrapCode(extract.KindExtractionFailed, "selector_candidate_missing", "selected source is missing", false, nil)
	}
	if strings.TrimSpace(req.CookieHeader) == "" {
		if existing := existingOutputPath(outputBase); existing != "" {
			return &downloadedArtifact{Path: existing, Method: "reuse_existing"}, nil
		}
	}

	if s.downloader == nil {
		return nil, extract.WrapCode(extract.KindInternal, "downloader_unavailable", "downloader is unavailable", false, nil)
	}

	result, err := s.downloader.Download(ctx, DownloadRequest{
		URL:          candidate.URL,
		Filename:     filepath.Base(outputBase),
		UserAgent:    req.UserAgent,
		CookieHeader: netutil.CookieForSameHost(req.CookieHeader, pageURL, candidate.URL),
		Referer:      candidate.Referer,
		Origin:       candidate.Origin,
		TempRoot:     filepath.Dir(outputBase),
	}, update)

	if err != nil {
		return nil, err
	}
	return &downloadedArtifact{Path: result.FilePath, Method: result.Method}, nil
}


func (s *Merger) preflightCandidate(ctx context.Context, candidate *Candidate, req MergeRequest) error {
	if candidate == nil || s.downloader == nil || !s.shouldUseDirectCandidate(candidate) {
		return nil
	}
	_, err := s.downloader.Preflight(ctx, DownloadRequest{URL: candidate.URL, Filename: "preflight", UserAgent: req.UserAgent, CookieHeader: netutil.CookieForSameHost(req.CookieHeader, req.URL, candidate.URL), Referer: candidate.Referer, Origin: candidate.Origin})
	return err
}

func (s *Merger) extractFromSource(ctx context.Context, rawURL, cookie string) (*extract.Result, error) {
	if s.extractor != nil {
		return s.extractor.Extract(ctx, rawURL, extract.ExtractOptions{CookieHeader: cookie, UseAuth: strings.TrimSpace(cookie) != ""})
	}
	return s.metadata.Extract(ctx, rawURL, extract.ExtractOptions{CookieHeader: cookie, UseAuth: strings.TrimSpace(cookie) != ""})
}

func (s *Merger) downloadSelectionPair(ctx context.Context, req MergeRequest, tmpDir string, selection *Selection, update func(storage.JobUpdate)) (*downloadedArtifact, *downloadedArtifact, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type pairResult struct {
		kind     string
		artifact *downloadedArtifact
		err      error
	}
	results := make(chan pairResult, 2)
	run := func(kind string, outputBase string, candidate *Candidate) {
		go func() {
			artifact, err := s.downloadCandidate(ctx, req.URL, outputBase, candidate, req, update)
			results <- pairResult{kind: kind, artifact: artifact, err: err}
		}()
	}
	run("video", filepath.Join(tmpDir, "video"), selection.Video)
	run("audio", filepath.Join(tmpDir, "audio"), selection.Audio)
	var videoArtifact, audioArtifact *downloadedArtifact
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			cancel()
			if videoArtifact != nil {
				cleanupDownloadedPath(videoArtifact.Path)
			}
			if audioArtifact != nil {
				cleanupDownloadedPath(audioArtifact.Path)
			}
			return nil, nil, result.err
		}
		if result.kind == "video" {
			videoArtifact = result.artifact
		} else {
			audioArtifact = result.artifact
		}
	}
	if videoArtifact == nil || audioArtifact == nil || strings.TrimSpace(videoArtifact.Path) == "" || strings.TrimSpace(audioArtifact.Path) == "" {
		if videoArtifact != nil {
			cleanupDownloadedPath(videoArtifact.Path)
		}
		if audioArtifact != nil {
			cleanupDownloadedPath(audioArtifact.Path)
		}
		return nil, nil, extract.WrapCode(extract.KindDownloadFailed, "download_pair_incomplete", "failed to download media pair", true, nil)
	}
	return videoArtifact, audioArtifact, nil
}

func joinDownloadMethods(values ...string) string {
	parts := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
	}
	return strings.Join(parts, "+")
}

func (s *Merger) shouldUseDirectCandidate(candidate *Candidate) bool {
	if candidate == nil || strings.TrimSpace(candidate.URL) == "" {
		return false
	}
	if strings.TrimSpace(candidate.FormatID) != "" && prefersYTDLPCandidateURL(candidate.URL) {
		return false
	}
	protocol := strings.ToLower(strings.TrimSpace(candidate.Protocol))
	return protocol == "" || protocol == "https" || protocol == "http"
}

func prefersYTDLPCandidateURL(rawURL string) bool {
	value := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.Contains(value, "googlevideo.com") || strings.Contains(value, "youtube.com") || strings.Contains(value, "youtu.be")
}

func cleanupDownloadedPath(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.RemoveAll(filepath.Dir(path))
}
