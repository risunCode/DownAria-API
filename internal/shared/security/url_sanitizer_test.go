package security

import "testing"

func TestSanitizeHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trim and lowercase scheme host", input: "  HTTPS://EXAMPLE.COM/path?q=1#frag  ", want: "https://example.com/path?q=1", wantErr: false},
		{name: "keep explicit port", input: "http://Example.com:8080/video", want: "http://example.com:8080/video", wantErr: false},
		{name: "reject empty", input: "", wantErr: true},
		{name: "reject unsupported scheme", input: "ftp://example.com/file", wantErr: true},
		{name: "reject missing host", input: "https:///x", wantErr: true},
		{name: "reject userinfo", input: "https://user:pass@example.com/x", wantErr: true},
		{name: "reject invalid port", input: "https://example.com:70000/x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := SanitizeHTTPURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SanitizeHTTPURL(%q) err=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if parsed == nil {
				t.Fatalf("SanitizeHTTPURL(%q) returned nil URL", tt.input)
			}
			if got := parsed.String(); got != tt.want {
				t.Fatalf("SanitizeHTTPURL(%q) got=%q want=%q", tt.input, got, tt.want)
			}
		})
	}
}
