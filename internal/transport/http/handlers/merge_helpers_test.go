package handlers

import "testing"

func TestIsAudioOnlyRequest(t *testing.T) {
	tests := []struct {
		name string
		req  mergeRequest
		want bool
	}{
		{name: "mp3 format", req: mergeRequest{Format: "mp3"}, want: true},
		{name: "m4a format", req: mergeRequest{Format: "m4a"}, want: true},
		{name: "quality mp3", req: mergeRequest{Quality: "MP3"}, want: true},
		{name: "video quality", req: mergeRequest{Quality: "1080p", Format: "mp4"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAudioOnlyRequest(tt.req)
			if got != tt.want {
				t.Fatalf("isAudioOnlyRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAudioOutput(t *testing.T) {
	ext, codec, contentType := resolveAudioOutput("m4a", "")
	if ext != "m4a" || codec != "aac" || contentType != "audio/mp4" {
		t.Fatalf("unexpected m4a output: %s %s %s", ext, codec, contentType)
	}

	ext, codec, contentType = resolveAudioOutput("", "mp3")
	if ext != "mp3" || codec != "libmp3lame" || contentType != "audio/mpeg" {
		t.Fatalf("unexpected mp3 output: %s %s %s", ext, codec, contentType)
	}
}

func TestEnsureFileExtension(t *testing.T) {
	if got := ensureFileExtension("", "mp3"); got != "downaria_output.mp3" {
		t.Fatalf("default filename mismatch: %s", got)
	}

	if got := ensureFileExtension("song", "mp3"); got != "song.mp3" {
		t.Fatalf("append ext mismatch: %s", got)
	}

	if got := ensureFileExtension("song.mp4", "mp3"); got != "song.mp3" {
		t.Fatalf("replace ext mismatch: %s", got)
	}

	if got := ensureFileExtension("song.mp3", "mp3"); got != "song.mp3" {
		t.Fatalf("keep ext mismatch: %s", got)
	}

	if got := ensureFileExtension("my_clip_HD.mp4", "mp4"); got != "my_clip.mp4" {
		t.Fatalf("strip HD suffix mismatch: %s", got)
	}

	if got := ensureFileExtension("my_clip (Original).mp4", "mp4"); got != "my_clip.mp4" {
		t.Fatalf("strip Original suffix mismatch: %s", got)
	}

	if got := ensureFileExtension("voice-audio", "mp3"); got != "voice.mp3" {
		t.Fatalf("strip audio suffix mismatch: %s", got)
	}

	if got := ensureFileExtension("HD", "mp4"); got != "downaria_output.mp4" {
		t.Fatalf("fallback filename mismatch after stripping labels: %s", got)
	}

	if got := ensureFileExtension("clip_[DownAria].mp4", "mp4"); got != "clip_[DownAria].mp4" {
		t.Fatalf("branded suffix should be preserved: %s", got)
	}

	if got := ensureFileExtension("Ã¨ÂµÂ¤Ã©Â_Â_jo._Ã¦Â±Â_Ã¦Â±Â_Ã¯Â½Â__126715010_[DownAria", "mp4"); got != "jo._126715010_[DownAria].mp4" {
		t.Fatalf("mojibake filename should be sanitized and tag normalized: %s", got)
	}
}

func TestBuildYTDLPFormatSelectorPrefersAVC1(t *testing.T) {
	sel := buildYTDLPFormatSelector("1080p", false)
	if sel == "" {
		t.Fatal("expected non-empty selector")
	}
	if got, want := sel, "bestvideo[vcodec^=avc1][height<=1080]+bestaudio/bestvideo[vcodec^=h264][height<=1080]+bestaudio/bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"; got != want {
		t.Fatalf("unexpected selector: %s", got)
	}
}

func TestBuildYTDLPFormatSelectorFallsBackToBest(t *testing.T) {
	sel := buildYTDLPFormatSelector("", false)
	if got, want := sel, "bestvideo[vcodec^=avc1]+bestaudio/bestvideo[vcodec^=h264]+bestaudio/bestvideo+bestaudio/best"; got != want {
		t.Fatalf("unexpected selector: %s", got)
	}
}

func TestResolveDownloadFilenameFallbackCompatibility(t *testing.T) {
	if got := resolveDownloadFilename("", "", "", "", "video/mp4"); got != "downaria_download.mp4" {
		t.Fatalf("default fallback filename mismatch: %s", got)
	}

	if got := resolveDownloadFilename("", "", "", "youtube", "video/mp4"); got != "downaria_youtube_download.mp4" {
		t.Fatalf("platform fallback filename mismatch: %s", got)
	}

	if got := resolveDownloadFilename("", "", "https://cdn.example.com/media/sample.webm", "", ""); got != "downaria_download.webm" {
		t.Fatalf("url extension fallback mismatch: %s", got)
	}
}

func TestResolveDownloadFilenamePreservesDownAriaBranding(t *testing.T) {
	if got := resolveDownloadFilename("", `attachment; filename="clip_[DownAria].mp4"`, "", "", "video/mp4"); got != "clip_[DownAria].mp4" {
		t.Fatalf("expected branded upstream filename to be preserved: %s", got)
	}
}
