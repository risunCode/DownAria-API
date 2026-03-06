package handlers

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"downaria-api/internal/extractors/core"
)

var (
	noiseURLRe        = regexp.MustCompile(`(?i)https?://\S+|www\.\S+`)
	noiseTagRe        = regexp.MustCompile(`(?i)(^|\s)[#@][\p{L}\p{N}_]+`)
	noiseSpaceRe      = regexp.MustCompile(`\s+`)
	filenameStopwords = map[string]struct{}{
		"follow": {}, "following": {}, "followers": {}, "subscribe": {}, "sub": {}, "like": {}, "likes": {}, "share": {},
		"repost": {}, "retweet": {}, "comment": {}, "comments": {}, "credit": {}, "credits": {}, "source": {},
		"original": {}, "link": {}, "bio": {}, "astag": {}, "hashtag": {}, "hashtags": {}, "trend": {}, "viral": {},
	}
)

func (h *Handler) ensureVariantFilenames(result *core.ExtractResult) {
	if result == nil {
		return
	}

	author := preferredAuthorSeed(result)
	title := smartTitleSeed(result)
	sequence := 0
	for mediaIdx := range result.Media {
		for variantIdx := range result.Media[mediaIdx].Variants {
			variant := &result.Media[mediaIdx].Variants[variantIdx]
			ext := inferVariantExtension(variant, result.Media[mediaIdx].Type)
			variant.Filename = generateDisplayFilename(author, title, ext, sequence)
			sequence++
		}
	}
}

func smartTitleSeed(result *core.ExtractResult) string {
	if result == nil {
		return ""
	}
	raw := strings.TrimSpace(result.Content.Text)
	if raw == "" {
		raw = strings.TrimSpace(result.Content.Description)
	}
	if raw == "" {
		return ""
	}

	clean := noiseURLRe.ReplaceAllString(raw, " ")
	clean = noiseTagRe.ReplaceAllString(clean, " ")
	clean = strings.NewReplacer("|", " ", "•", " ", "·", " ", "—", " ", "-", " ").Replace(clean)
	clean = noiseSpaceRe.ReplaceAllString(strings.TrimSpace(clean), " ")
	if clean == "" {
		return raw
	}

	tokens := strings.Fields(clean)
	filtered := make([]string, 0, len(tokens))
	for _, tk := range tokens {
		v := strings.Trim(strings.ToLower(strings.TrimSpace(tk)), "_.,:;!?()[]{}\"'`~")
		if v == "" {
			continue
		}
		if _, blocked := filenameStopwords[v]; blocked {
			continue
		}
		if utf8.RuneCountInString(v) <= 1 {
			continue
		}
		filtered = append(filtered, tk)
	}

	if len(filtered) == 0 {
		return clean
	}

	return strings.Join(filtered, " ")
}

func generateDisplayFilename(author, title, ext string, mediaIdx int) string {
	indexToken := "0"
	if mediaIdx > 0 {
		indexToken = strconv.Itoa(mediaIdx)
	}
	return core.GenerateFilename(author, title, indexToken, ext)
}

func preferredAuthorSeed(result *core.ExtractResult) string {
	if result == nil {
		return "media"
	}
	handle := strings.TrimSpace(result.Author.Handle)
	if handle != "" && !isUnknownSeed(handle) {
		return strings.TrimPrefix(handle, "@")
	}
	author := strings.TrimSpace(result.Author.Name)
	if author != "" && !isUnknownSeed(author) {
		return author
	}
	title := strings.TrimSpace(result.Content.Text)
	if title == "" {
		title = strings.TrimSpace(result.Content.Description)
	}
	if title != "" {
		return title
	}
	return "media"
}

func isUnknownSeed(value string) bool {
	v := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "@")))
	return v == "unknown" || v == "n/a" || v == "na" || v == "none"
}

func inferVariantExtension(variant *core.Variant, mediaType core.MediaType) string {
	if variant == nil {
		return defaultExtensionForMediaType(mediaType)
	}

	if format := strings.TrimSpace(variant.Format); format != "" {
		return strings.TrimPrefix(strings.ToLower(format), ".")
	}

	if ext := core.GetExtensionFromMime(variant.Mime); ext != "bin" {
		return ext
	}

	if guessed := extensionFromURL(variant.URL); guessed != "" {
		return guessed
	}

	return defaultExtensionForMediaType(mediaType)
}

func extensionFromURL(rawURL string) string {
	v := strings.TrimSpace(rawURL)
	if v == "" {
		return ""
	}
	base := path.Base(v)
	if idx := strings.Index(base, "?"); idx >= 0 {
		base = base[:idx]
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(base)), ".")
	if ext == "" {
		return ""
	}
	return ext
}

func defaultExtensionForMediaType(mediaType core.MediaType) string {
	switch mediaType {
	case core.MediaTypeVideo, core.MediaTypeReel, core.MediaTypeStory:
		return "mp4"
	case core.MediaTypeAudio:
		return "mp3"
	case core.MediaTypeImage:
		return "jpg"
	default:
		return "bin"
	}
}
