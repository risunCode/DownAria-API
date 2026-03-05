package network

import "testing"

func TestBufferPoolAdaptiveSize(t *testing.T) {
	bp := NewBufferPool()
	if got := bp.SizeForContentType("video/mp4"); got != VideoBufferSize {
		t.Fatalf("video size mismatch: %d", got)
	}
	if got := bp.SizeForContentType("audio/mpeg"); got != AudioBufferSize {
		t.Fatalf("audio size mismatch: %d", got)
	}
	bp.SetMemoryPressure(true)
	if got := bp.SizeForContentType("video/mp4"); got != VideoBufferSize/2 {
		t.Fatalf("expected halved size, got %d", got)
	}
}

func TestBufferPoolReuse(t *testing.T) {
	bp := NewBufferPool()
	b := bp.Get(1024)
	b[0] = 1
	bp.Put(b)
	b2 := bp.Get(1024)
	if len(b2) != 1024 {
		t.Fatalf("unexpected len: %d", len(b2))
	}
}
