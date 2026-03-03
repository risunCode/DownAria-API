package extractors

import (
	"regexp"

	ariaextended "fetchmoona/internal/extractors/aria-extended"
	extcore "fetchmoona/internal/extractors/core"
	"fetchmoona/internal/extractors/native/facebook"
	"fetchmoona/internal/extractors/native/instagram"
	"fetchmoona/internal/extractors/native/pixiv"
	"fetchmoona/internal/extractors/native/threads"
	"fetchmoona/internal/extractors/native/tiktok"
	"fetchmoona/internal/extractors/native/twitter"
	"fetchmoona/internal/extractors/registry"
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
