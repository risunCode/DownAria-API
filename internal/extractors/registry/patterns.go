package registry

import (
	"regexp"
)

// URL Patterns for all platforms

var FacebookPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.|web\.|m\.|mbasic\.)?facebook\.com/`),
	regexp.MustCompile(`(?i)^https?://fb\.watch/`),
	regexp.MustCompile(`(?i)^https?://fb\.me/`),
}

var InstagramPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.)?instagram\.com/`),
	regexp.MustCompile(`(?i)^https?://instagr\.am/`),
}

var ThreadsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.)?threads\.net/`),
	regexp.MustCompile(`(?i)^https?://(?:www\.)?threads\.com/`),
}

var TwitterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.)?twitter\.com/`),
	regexp.MustCompile(`(?i)^https?://(?:www\.)?x\.com/`),
	regexp.MustCompile(`(?i)^https?://t\.co/`),
}

var TikTokPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.)?tiktok\.com/`),
	regexp.MustCompile(`(?i)^https?://vm\.tiktok\.com/`),
	regexp.MustCompile(`(?i)^https?://vt\.tiktok\.com/`),
}

var YouTubePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:[a-z0-9-]+\.)?youtube\.com/(?:watch\?|shorts/|live/|embed/|v/|clip/)`),
	regexp.MustCompile(`(?i)^https?://youtu\.be/`),
}

var PixivPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^https?://(?:www\.)?pixiv\.net/`),
}
