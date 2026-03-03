package security

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactLogValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		notHas   []string
	}{
		{
			name:     "redacts query string from absolute url",
			input:    "upstream=https://cdn.example.com/video.mp4?token=abc123&quality=hd",
			contains: []string{"https://cdn.example.com/video.mp4"},
			notHas:   []string{"token=abc123", "quality=hd"},
		},
		{
			name:     "redacts authorization header",
			input:    "Authorization: Bearer super-secret-token",
			contains: []string{"Authorization: [REDACTED]"},
			notHas:   []string{"super-secret-token"},
		},
		{
			name:     "redacts upstream auth query",
			input:    "url=/api/proxy?upstream_auth=abc.def.ghi&foo=1",
			contains: []string{"upstream_auth=[REDACTED]", "foo=1"},
			notHas:   []string{"abc.def.ghi"},
		},
		{
			name:     "redacts cookie header",
			input:    "cookie=sessionid=s3cr3t; theme=dark",
			contains: []string{"cookie=[REDACTED]"},
			notHas:   []string{"sessionid=s3cr3t"},
		},
		{
			name:     "redacts bearer token text",
			input:    "auth failed for Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			contains: []string{"Bearer [REDACTED]"},
			notHas:   []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redacted := RedactLogValue(tt.input)
			for _, expected := range tt.contains {
				if !strings.Contains(redacted, expected) {
					t.Fatalf("expected %q to contain %q", redacted, expected)
				}
			}
			for _, rejected := range tt.notHas {
				if strings.Contains(redacted, rejected) {
					t.Fatalf("expected %q to not contain %q", redacted, rejected)
				}
			}
		})
	}
}

func TestRedactLogError(t *testing.T) {
	err := errors.New("request failed: Authorization: Bearer secret-token")
	redacted := RedactLogError(err)
	if !strings.Contains(redacted, "Authorization: [REDACTED]") {
		t.Fatalf("unexpected redaction result: %q", redacted)
	}
	if strings.Contains(redacted, "secret-token") {
		t.Fatalf("sensitive token leaked in redaction result: %q", redacted)
	}
}
