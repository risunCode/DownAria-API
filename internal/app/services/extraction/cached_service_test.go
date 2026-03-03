package extraction

import (
	"context"
	"testing"
	"time"

	"downaria-api/internal/extractors/core"
	"downaria-api/internal/infra/cache"
)

type mockCachedNextService struct {
	result *core.ExtractResult
	calls  int
}

func (m *mockCachedNextService) Extract(_ context.Context, _ ExtractInput) (*core.ExtractResult, error) {
	m.calls++
	if m.result == nil {
		return nil, nil
	}
	cloned := *m.result
	return &cloned, nil
}

func TestCachedService_UsesPlatformSpecificTTL(t *testing.T) {
	next := &mockCachedNextService{result: &core.ExtractResult{URL: "https://x.com/post/1", Platform: "twitter"}}

	svc := NewCachedService(next, cache.NewPlatformTTLConfig(200*time.Millisecond, map[string]time.Duration{
		"twitter": 30 * time.Millisecond,
	}))

	ctx := context.Background()
	input := ExtractInput{URL: "https://x.com/post/1"}

	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("first extract failed: %v", err)
	}
	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("second extract failed: %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("expected first two calls to hit upstream once, got %d", next.calls)
	}

	time.Sleep(50 * time.Millisecond)

	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("third extract failed: %v", err)
	}
	if next.calls != 2 {
		t.Fatalf("expected cache to expire using twitter ttl, upstream calls=%d", next.calls)
	}
}

func TestCachedService_FallsBackToDefaultTTL(t *testing.T) {
	next := &mockCachedNextService{result: &core.ExtractResult{URL: "https://unknown.example/video/1", Platform: "unknown-platform"}}

	svc := NewCachedService(next, cache.NewPlatformTTLConfig(90*time.Millisecond, map[string]time.Duration{
		"twitter": 20 * time.Millisecond,
	}))

	ctx := context.Background()
	input := ExtractInput{URL: "https://unknown.example/video/1"}

	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("first extract failed: %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("second extract failed: %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("expected default ttl to keep entry cached, upstream calls=%d", next.calls)
	}

	time.Sleep(70 * time.Millisecond)

	if _, err := svc.Extract(ctx, input); err != nil {
		t.Fatalf("third extract failed: %v", err)
	}
	if next.calls != 2 {
		t.Fatalf("expected cache to expire using default ttl, upstream calls=%d", next.calls)
	}
}
