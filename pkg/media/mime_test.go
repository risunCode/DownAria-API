package media

import "testing"

func TestGetExtensionFromMime_NormalizesAndMapsCommonTypes(t *testing.T) {
	tests := []struct {
		name string
		mime string
		want string
	}{
		{name: "hls with params", mime: "application/vnd.apple.mpegurl; charset=utf-8", want: "m3u8"},
		{name: "dash", mime: "application/dash+xml", want: "mpd"},
		{name: "wav alias", mime: "audio/x-wav", want: "wav"},
		{name: "audio mp4", mime: "audio/mp4; codecs=mp4a.40.2", want: "m4a"},
		{name: "unknown", mime: "application/octet-stream", want: "bin"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := GetExtensionFromMime(tc.mime); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestGetMimeFromExtension_MapsAdditionalTypes(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{ext: "mkv", want: "video/x-matroska"},
		{ext: ".ts", want: "video/mp2t"},
		{ext: "aac", want: "audio/aac"},
		{ext: "svg", want: "image/svg+xml"},
		{ext: "m3u", want: "application/x-mpegurl"},
	}

	for _, tc := range tests {
		if got := GetMimeFromExtension(tc.ext); got != tc.want {
			t.Fatalf("ext %q expected %q, got %q", tc.ext, tc.want, got)
		}
	}
}
