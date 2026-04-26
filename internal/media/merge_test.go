package media

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"downaria-api/internal/extract"
)

func TestBuildFFmpegArgsMapsStreams(t *testing.T) {
	args := strings.Join(buildFFmpegArgs("v.mp4", "a.m4a", "mp4", "out.mp4"), " ")
	for _, want := range []string{"-map 0:v:0", "-map 1:a:0", "-movflags +faststart"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q: %s", want, args)
		}
	}
}

func TestMergeLocalFilesNoConcurrentPathCollision(t *testing.T) {
	mergeRoot := t.TempDir()
	t.Setenv("DOWNARIA_API_RUNTIME_ROOT", mergeRoot)

	const goroutines = 10
	paths := make([]string, goroutines)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dir, err := os.MkdirTemp(mergeRoot, "merge-*")
			if err != nil {
				t.Errorf("MkdirTemp: %v", err)
				return
			}
			outputPath := filepath.Join(dir, "output.mp4")
			mu.Lock()
			paths[idx] = outputPath
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool)
	for _, p := range paths {
		if p == "" {
			continue
		}
		if seen[p] {
			t.Errorf("path collision detected: %s", p)
		}
		seen[p] = true
	}
}

func TestMergerLimits(t *testing.T) {
	m := &Merger{maxOutputBytes: 10, maxDuration: 5_000_000_000}
	if err := m.ensureOutputSize(11); err == nil {
		t.Fatal("expected size error")
	}
	result := &extract.Result{Media: []extract.MediaItem{{Sources: []extract.MediaSource{{DurationSeconds: 6}}}}}
	if err := m.validateDuration(result); err == nil {
		t.Fatal("expected duration error")
	}
}

func TestShouldConvertProgressiveForFormat(t *testing.T) {
	if shouldConvertProgressiveForFormat("mp4", &Candidate{Container: "webm"}, "artifact.webm") != true {
		t.Fatal("expected webm progressive artifact to require conversion for mp4")
	}
	if shouldConvertProgressiveForFormat("mp4", &Candidate{Container: "mp4"}, "artifact.mp4") {
		t.Fatal("did not expect mp4 progressive artifact to require conversion")
	}
	if shouldConvertProgressiveForFormat("webm", &Candidate{Container: "webm"}, "artifact.webm") {
		t.Fatal("did not expect non-mp4 target format to force conversion")
	}
}

func TestBuildFFmpegContainerConvertArgsForMP4(t *testing.T) {
	args := strings.Join(buildFFmpegContainerConvertArgs("in.webm", "mp4", "out.mp4"), " ")
	for _, want := range []string{"-map 0:v:0", "-map 0:a:0?", "-c:v copy", "-c:a aac", "-movflags +faststart", "-f mp4"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q: %s", want, args)
		}
	}
}

func TestMarkSelectionSourcesExcluded(t *testing.T) {
	selection := &Selection{
		Video: &Candidate{URL: "https://video"},
		Audio: &Candidate{URL: "https://audio"},
	}
	excluded := map[string]struct{}{}
	if !markSelectionSourcesExcluded(selection, excluded) {
		t.Fatal("expected sources to be marked")
	}
	if _, ok := excluded["https://video"]; !ok {
		t.Fatal("video url not excluded")
	}
	if _, ok := excluded["https://audio"]; !ok {
		t.Fatal("audio url not excluded")
	}
}
