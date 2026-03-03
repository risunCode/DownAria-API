package core

// ResponseBuilder builds consistent extraction responses
type ResponseBuilder struct {
	result ExtractResult
}

// NewResponseBuilder creates a new builder with URL
func NewResponseBuilder(url string) *ResponseBuilder {
	return &ResponseBuilder{
		result: ExtractResult{
			URL:       url,
			MediaType: MediaTypeUnknown,
			Author:    Author{},
			Content:   Content{},
			Engagement: Engagement{
				Views:     0,
				Likes:     0,
				Comments:  0,
				Shares:    0,
				Bookmarks: 0,
			},
			Media:          []Media{},
			Authentication: Authentication{Used: false, Source: AuthSourceNone},
		},
	}
}

// WithPlatform sets the platform
func (b *ResponseBuilder) WithPlatform(platform string) *ResponseBuilder {
	b.result.Platform = platform
	return b
}

// WithMediaType sets the media type
func (b *ResponseBuilder) WithMediaType(mt MediaType) *ResponseBuilder {
	b.result.MediaType = mt
	return b
}

// WithAuthor sets author info
func (b *ResponseBuilder) WithAuthor(name, handle string) *ResponseBuilder {
	b.result.Author = Author{Name: name, Handle: handle}
	return b
}

// WithContent sets content info
func (b *ResponseBuilder) WithContent(id, text, description string) *ResponseBuilder {
	b.result.Content = Content{
		ID:          id,
		Text:        text,
		Description: description,
	}
	return b
}

// WithCreatedAt sets creation date
func (b *ResponseBuilder) WithCreatedAt(createdAt string) *ResponseBuilder {
	b.result.Content.CreatedAt = createdAt
	return b
}

// WithEngagement sets engagement stats
func (b *ResponseBuilder) WithEngagement(views, likes, comments, shares int64) *ResponseBuilder {
	b.result.Engagement = Engagement{
		Views:     views,
		Likes:     likes,
		Comments:  comments,
		Shares:    shares,
		Bookmarks: 0,
	}
	return b
}

// WithBookmarks sets bookmark stats (used by platforms like Pixiv)
func (b *ResponseBuilder) WithBookmarks(bookmarks int64) *ResponseBuilder {
	b.result.Engagement.Bookmarks = bookmarks
	return b
}

// WithAuthentication sets auth info
func (b *ResponseBuilder) WithAuthentication(used bool, source AuthSource) *ResponseBuilder {
	b.result.Authentication = Authentication{Used: used, Source: source}
	return b
}

// AddMedia adds a media item
func (b *ResponseBuilder) AddMedia(media Media) *ResponseBuilder {
	b.result.Media = append(b.result.Media, media)
	return b
}

// Build returns the final result
func (b *ResponseBuilder) Build() *ExtractResult {
	return &b.result
}

// Helper functions for creating Media and Variants

// NewMedia creates a new Media item
func NewMedia(index int, mediaType MediaType, thumbnail string) Media {
	return Media{
		Index:     index,
		Type:      mediaType,
		Thumbnail: thumbnail,
		Variants:  []Variant{},
	}
}

// AddVariant adds a variant to media (modifies the media in place)
func AddVariant(m *Media, v Variant) {
	m.Variants = append(m.Variants, v)
}

// NewVariant creates a new variant
func NewVariant(quality, url string) Variant {
	return Variant{
		Quality: quality,
		URL:     url,
	}
}

// WithFormat sets format (file extension)
func (v Variant) WithFormat(format string) Variant {
	v.Format = format
	return v
}

// WithMime sets MIME type
func (v Variant) WithMime(mime string) Variant {
	v.Mime = mime
	return v
}

// WithSize sets file size
func (v Variant) WithSize(size int64) Variant {
	v.Size = size
	return v
}

// WithResolution sets resolution
func (v Variant) WithResolution(res string) Variant {
	v.Resolution = res
	return v
}

// WithCodec sets codec
func (v Variant) WithCodec(codec string) Variant {
	v.Codec = codec
	return v
}

// WithAudio sets hasAudio flag
func (v Variant) WithAudio(hasAudio bool) Variant {
	v.HasAudio = hasAudio
	return v
}

// WithMerge sets requiresMerge flag
func (v Variant) WithMerge(requiresMerge bool) Variant {
	v.RequiresMerge = requiresMerge
	return v
}

// WithProxy sets requiresProxy flag
func (v Variant) WithProxy(requiresProxy bool) Variant {
	v.RequiresProxy = requiresProxy
	return v
}

// WithFormatID sets internal format ID
func (v Variant) WithFormatID(id string) Variant {
	v.FormatID = id
	return v
}

// WithBitrate sets bitrate
func (v Variant) WithBitrate(bitrate int) Variant {
	v.Bitrate = bitrate
	return v
}
