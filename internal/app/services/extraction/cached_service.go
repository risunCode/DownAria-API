package extraction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"fetchmoona/internal/extractors/core"
	"fetchmoona/internal/infra/cache"
	"golang.org/x/sync/singleflight"
)

type cachedService struct {
	next      Service
	cache     *cache.TTLCache
	ttlConfig cache.PlatformTTLConfig
	missGroup singleflight.Group
}

func NewCachedService(next Service, ttlConfig cache.PlatformTTLConfig) Service {
	return &cachedService{
		next:      next,
		cache:     cache.NewTTLCache(),
		ttlConfig: cache.NewPlatformTTLConfig(ttlConfig.DefaultTTL, ttlConfig.PlatformTTLs),
	}
}

func (s *cachedService) Extract(ctx context.Context, input ExtractInput) (*core.ExtractResult, error) {
	key := buildExtractCacheKey(input.URL, input.Cookie)
	if value, ok := s.cache.Get(key); ok {
		if result, castOK := value.(*core.ExtractResult); castOK && result != nil {
			return cloneExtractResult(result), nil
		}
	}

	v, err, _ := s.missGroup.Do(key, func() (any, error) {
		if value, ok := s.cache.Get(key); ok {
			if result, castOK := value.(*core.ExtractResult); castOK && result != nil {
				return cloneExtractResult(result), nil
			}
		}

		result, extractErr := s.next.Extract(ctx, input)
		if extractErr != nil {
			return nil, extractErr
		}

		if result == nil {
			return (*core.ExtractResult)(nil), nil
		}

		stored := cloneExtractResult(result)
		s.cache.Set(key, stored, s.ttlConfig.TTLForPlatform(stored.Platform))
		return cloneExtractResult(stored), nil
	})
	if err != nil {
		return nil, err
	}

	result, _ := v.(*core.ExtractResult)
	if result == nil {
		return nil, nil
	}
	return cloneExtractResult(result), nil
}

func buildExtractCacheKey(rawURL string, rawCookie string) string {
	urlPart := strings.TrimSpace(rawURL)
	cookiePart := strings.TrimSpace(rawCookie)
	cookieHash := sha256.Sum256([]byte(cookiePart))

	return fmt.Sprintf("url=%s|cookie_present=%t|cookie_sha256=%s", urlPart, cookiePart != "", hex.EncodeToString(cookieHash[:]))
}

func cloneExtractResult(src *core.ExtractResult) *core.ExtractResult {
	if src == nil {
		return nil
	}

	cloned := *src
	if src.Media != nil {
		cloned.Media = make([]core.Media, len(src.Media))
		for i := range src.Media {
			cloned.Media[i] = src.Media[i]
			if src.Media[i].Variants != nil {
				cloned.Media[i].Variants = make([]core.Variant, len(src.Media[i].Variants))
				copy(cloned.Media[i].Variants, src.Media[i].Variants)
			}
		}
	}

	return &cloned
}
