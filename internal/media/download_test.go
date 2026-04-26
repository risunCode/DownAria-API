package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHeadersStripsControlChars(t *testing.T) {
	headers := buildHeaders(DownloadRequest{UserAgent: "good\r\nbad", CookieHeader: "a=b\nInjected: x"})
	for key, value := range headers {
		if strings.ContainsAny(key+value, "\r\n\x00") {
			t.Fatalf("unsafe header %q=%q", key, value)
		}
	}
}

func TestEnsureDownloadRootCreatesCustomRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "downloads")
	got, err := ensureDownloadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q", got)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
}
