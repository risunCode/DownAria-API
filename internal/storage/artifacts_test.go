package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestArtifactStoreSaveGetDelete(t *testing.T) {
	store, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifact, err := store.SaveFile(src, "hello.txt", "text/plain", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Filename != "hello.txt" || got.ContentBytes != 5 {
		t.Fatalf("artifact = %#v", got)
	}
	if err := store.Delete(artifact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(artifact.ID); err == nil {
		t.Fatal("expected missing artifact")
	}
}

func TestArtifactStoreCloseStopsGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	store, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Allow goroutine to start
	time.Sleep(20 * time.Millisecond)
	if runtime.NumGoroutine() <= before {
		t.Fatal("expected cleanup goroutine to be running")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Allow goroutine to exit
	time.Sleep(20 * time.Millisecond)
	if runtime.NumGoroutine() > before {
		t.Errorf("expected goroutine count to return to baseline after Close()")
	}
	// Safe to call multiple times
	if err := store.Close(); err != nil {
		t.Fatal("second Close() should not error")
	}
}
