package extractors

import (
	"regexp"

	ariaextended "downaria-api/internal/extractors/aria-extended"
	extcore "downaria-api/internal/extractors/core"
	"downaria-api/internal/extractors/native/facebook"
	"downaria-api/internal/extractors/native/instagram"
	"downaria-api/internal/extractors/native/pixiv"
	"downaria-api/internal/extractors/native/threads"
	"downaria-api/internal/extractors/native/tiktok"
	"downaria-api/internal/extractors/native/twitter"
	"downaria-api/internal/extractors/registry"
)

type PlatformMetadata struct {
	Name         string
	Patterns     []*regexp.Regexp
	SupportsAuth bool
}

type PlatformDefinition struct {
	Metadata PlatformMetadata
	Factory  registry.ExtractorFactory
}

func DefaultPlatformDefinitions() []PlatformDefinition {
	return []PlatformDefinition{
		{
			Metadata: PlatformMetadata{
				Name:         "facebook",
				Patterns:     registry.FacebookPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return facebook.NewFacebookExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "instagram",
				Patterns:     registry.InstagramPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return instagram.NewInstagramExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "threads",
				Patterns:     registry.ThreadsPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return threads.NewThreadsExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "tiktok",
				Patterns:     registry.TikTokPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return tiktok.NewTikTokExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "twitter",
				Patterns:     registry.TwitterPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return twitter.NewTwitterExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "pixiv",
				Patterns:     registry.PixivPatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return pixiv.NewPixivExtractor()
			},
		},
		{
			Metadata: PlatformMetadata{
				Name:         "youtube",
				Patterns:     registry.YouTubePatterns,
				SupportsAuth: false,
			},
			Factory: func() extcore.Extractor {
				return ariaextended.NewPythonExtractor("youtube")
			},
		},
	}
}

func RegisterDefaultExtractors(reg *registry.Registry) {
	for _, def := range DefaultPlatformDefinitions() {
		reg.Register(def.Metadata.Name, def.Metadata.Patterns, def.Factory)
	}
}
