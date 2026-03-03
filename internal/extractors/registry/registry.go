package registry

import (
	"fmt"
	"regexp"
	"sync"

	"fetchmoona/internal/extractors/core"
)

type ExtractorFactory func() core.Extractor

type registeredExtractor struct {
	platform string
	patterns []*regexp.Regexp
	factory  ExtractorFactory
}

type Registry struct {
	extractors []registeredExtractor
	mu         sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		extractors: make([]registeredExtractor, 0),
	}
}

func (r *Registry) Register(platform string, patterns []*regexp.Regexp, factory ExtractorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.extractors = append(r.extractors, registeredExtractor{
		platform: platform,
		patterns: patterns,
		factory:  factory,
	})
}

func (r *Registry) GetExtractor(url string) (core.Extractor, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.extractors {
		for _, pattern := range e.patterns {
			if pattern.MatchString(url) {
				return e.factory(), e.platform, nil
			}
		}
	}

	return nil, "", fmt.Errorf("unsupported platform for URL: %s", url)
}
