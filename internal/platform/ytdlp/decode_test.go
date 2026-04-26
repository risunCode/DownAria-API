package ytdlp

import "testing"

func TestDecodeDumpRejectsEmptyOutput(t *testing.T) {
	if _, err := DecodeDump([]byte(`{}`)); err == nil {
		t.Fatal("expected empty output error")
	}
}

func TestMapResultProgressiveAndAdaptive(t *testing.T) {
	dump := &Dump{WebpageURL: "https://youtube.com/watch?v=x", Title: "video", Uploader: "me", Formats: []Format{
		{FormatID: "18", URL: "https://cdn.example.com/18.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "mp4a", Height: 720, FileSize: 100},
		{FormatID: "137", URL: "https://cdn.example.com/137.mp4", Ext: "mp4", VCodec: "avc1", ACodec: "none", Height: 1080, FileSize: 200},
		{FormatID: "140", URL: "https://cdn.example.com/140.m4a", Ext: "m4a", VCodec: "none", ACodec: "mp4a", FileSize: 50},
	}}
	result, err := MapResult(dump.WebpageURL, dump)
	if err != nil {
		t.Fatal(err)
	}
	if result.Platform == "" || len(result.Media) < 2 {
		t.Fatalf("result = %#v", result)
	}
}
