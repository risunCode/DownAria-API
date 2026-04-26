package media

import (
	"testing"

	"downaria-api/internal/extract"
)

func TestSelectPrefersProgressiveMP4(t *testing.T) {
	result := &extract.Result{Media: []extract.MediaItem{{Type: "video", Sources: []extract.MediaSource{{FormatID: "18", URL: "https://cdn/a.mp4", Container: "mp4", Height: 720, HasVideo: true, HasAudio: true, IsProgressive: true}}}}}
	selection, err := Select(result, "720p", "mp4")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != "progressive" || selection.Video.FormatID != "18" {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectSeparateVideoAudio(t *testing.T) {
	result := &extract.Result{Media: []extract.MediaItem{
		{Type: "video", Sources: []extract.MediaSource{{FormatID: "137", URL: "https://cdn/v.mp4", Container: "mp4", Height: 1080, HasVideo: true}}},
		{Type: "audio", Sources: []extract.MediaSource{{FormatID: "140", URL: "https://cdn/a.m4a", Container: "m4a", HasAudio: true}}},
	}}
	selection, err := Select(result, "1080p", "mp4")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != "separate" || selection.Video.FormatID != "137" || selection.Audio.FormatID != "140" {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestSelectPrefersDirectHTTPOverHLSForSameQuality(t *testing.T) {
	result := &extract.Result{Media: []extract.MediaItem{{Type: "video", Sources: []extract.MediaSource{
		{FormatID: "hls-720", URL: "https://cdn/hls.m3u8", Container: "mp4", Height: 720, HasVideo: true, HasAudio: true, IsProgressive: true, Protocol: "m3u8_native"},
		{FormatID: "http-720", URL: "https://cdn/direct.mp4", Container: "mp4", Height: 720, HasVideo: true, HasAudio: true, IsProgressive: true, Protocol: "https"},
	}}}}
	selection, err := Select(result, "720p", "mp4")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Video == nil || selection.Video.FormatID != "http-720" {
		t.Fatalf("expected direct http candidate, got %#v", selection)
	}
}
