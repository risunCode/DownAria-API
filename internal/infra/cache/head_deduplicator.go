package cache

import (
	"container/list"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type HeadMetadata struct {
	StatusCode    int
	ContentType   string
	ContentLength string
	FetchedAt     time.Time
}

type cacheItem struct {
	key       string
	value     HeadMetadata
	expiresAt time.Time
}

type HeadDeduplicator struct {
	client  *http.Client
	ttl     time.Duration
	maxSize int
	group   singleflight.Group
	mu      sync.Mutex
	items   map[string]*list.Element
	lru     *list.List
}

func NewHeadDeduplicator(client *http.Client, ttl time.Duration, maxSize int) *HeadDeduplicator {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	if maxSize <= 0 {
		maxSize = 2048
	}
	return &HeadDeduplicator{
		client:  client,
		ttl:     ttl,
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}
}

func (h *HeadDeduplicator) GetMetadata(ctx context.Context, targetURL string, headers map[string]string) (HeadMetadata, error) {
	key := buildHeadCacheKey(targetURL, headers)
	if meta, ok := h.getCached(key); ok {
		return meta, nil
	}

	v, err, _ := h.group.Do(key, func() (any, error) {
		if meta, ok := h.getCached(key); ok {
			return meta, nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
		if err != nil {
			return HeadMetadata{}, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := h.client.Do(req)
		if err != nil {
			return HeadMetadata{}, err
		}
		defer resp.Body.Close()

		meta := HeadMetadata{
			StatusCode:    resp.StatusCode,
			ContentType:   strings.TrimSpace(resp.Header.Get("Content-Type")),
			ContentLength: strings.TrimSpace(resp.Header.Get("Content-Length")),
			FetchedAt:     time.Now().UTC(),
		}

		if meta.StatusCode < http.StatusBadRequest {
			h.setCached(key, meta)
		}
		return meta, nil
	})
	if err != nil {
		return HeadMetadata{}, err
	}

	meta, ok := v.(HeadMetadata)
	if !ok {
		return HeadMetadata{}, fmt.Errorf("invalid head metadata type")
	}
	return meta, nil
}

func (h *HeadDeduplicator) getCached(key string) (HeadMetadata, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	elem, ok := h.items[key]
	if !ok {
		return HeadMetadata{}, false
	}
	item := elem.Value.(*cacheItem)
	if time.Now().After(item.expiresAt) {
		h.lru.Remove(elem)
		delete(h.items, key)
		return HeadMetadata{}, false
	}
	h.lru.MoveToFront(elem)
	return item.value, true
}

func (h *HeadDeduplicator) setCached(key string, value HeadMetadata) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if elem, ok := h.items[key]; ok {
		item := elem.Value.(*cacheItem)
		item.value = value
		item.expiresAt = time.Now().Add(h.ttl)
		h.lru.MoveToFront(elem)
		return
	}
	item := &cacheItem{key: key, value: value, expiresAt: time.Now().Add(h.ttl)}
	elem := h.lru.PushFront(item)
	h.items[key] = elem
	if len(h.items) > h.maxSize {
		oldest := h.lru.Back()
		if oldest != nil {
			old := oldest.Value.(*cacheItem)
			delete(h.items, old.key)
			h.lru.Remove(oldest)
		}
	}
}

func buildHeadCacheKey(targetURL string, headers map[string]string) string {
	if len(headers) == 0 {
		return strings.TrimSpace(targetURL)
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b := strings.Builder{}
	b.WriteString(strings.TrimSpace(targetURL))
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(strings.ToLower(strings.TrimSpace(k)))
		b.WriteString("=")
		b.WriteString(strings.TrimSpace(headers[k]))
	}
	return b.String()
}
