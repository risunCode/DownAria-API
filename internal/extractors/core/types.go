package core

import "context"

// MediaType represents the classification of content
type MediaType string

const (
	MediaTypeStory   MediaType = "story"
	MediaTypeReel    MediaType = "reel"
	MediaTypeVideo   MediaType = "video"
	MediaTypePost    MediaType = "post"
	MediaTypeImage   MediaType = "image"
	MediaTypeAudio   MediaType = "audio"
	MediaTypeUnknown MediaType = "unknown"
)

// AuthSource represents the source of authentication
type AuthSource string

const (
	AuthSourceNone   AuthSource = "none"
	AuthSourceServer AuthSource = "server"
	AuthSourceClient AuthSource = "client"
)

// ExtractOptions contains options for extraction
type ExtractOptions struct {
	Ctx     context.Context
	Cookie  string            // Cookie string for authentication
	Headers map[string]string // Additional headers
	Timeout int               // Timeout in seconds
	Source  AuthSource        // Source of authentication
}

// Variant represents a single media variant with quality info
type Variant struct {
	Quality       string `json:"quality"`                 // Quality label (e.g., "720p", "Original")
	URL           string `json:"url"`                     // Direct URL to media
	Resolution    string `json:"resolution,omitempty"`    // Resolution string (e.g., "1920x1080")
	Mime          string `json:"mime,omitempty"`          // MIME type
	Format        string `json:"format,omitempty"`        // File extension
	Size          int64  `json:"size,omitempty"`          // File size in bytes
	Bitrate       int    `json:"bitrate,omitempty"`       // Bitrate in kbps
	Codec         string `json:"codec,omitempty"`         // Video/audio codec
	HasAudio      bool   `json:"hasAudio,omitempty"`      // Whether this variant has audio
	RequiresMerge bool   `json:"requiresMerge,omitempty"` // Whether video needs audio merge
	RequiresProxy bool   `json:"requiresProxy,omitempty"` // Whether URL needs proxying
	FormatID      string `json:"formatId,omitempty"`      // Internal format identifier
}

// Media represents a media item with multiple variants
type Media struct {
	Index     int       `json:"index"`               // Index in media array
	Type      MediaType `json:"type"`                // Media type (video/image/audio)
	Thumbnail string    `json:"thumbnail,omitempty"` // Thumbnail URL
	Variants  []Variant `json:"variants"`            // Available quality variants
}

// Author represents content creator information
type Author struct {
	Name   string `json:"name,omitempty"`   // Display name
	Handle string `json:"handle,omitempty"` // Username/handle (e.g., @username)
}

// Content represents the extracted content metadata
type Content struct {
	ID          string `json:"id,omitempty"`          // Platform-specific content ID
	Text        string `json:"text,omitempty"`        // Primary text (title/caption)
	Description string `json:"description,omitempty"` // Secondary text/description
	CreatedAt   string `json:"createdAt,omitempty"`   // ISO 8601 creation date
}

// Engagement represents social engagement metrics
type Engagement struct {
	Views     int64 `json:"views"`     // View count
	Likes     int64 `json:"likes"`     // Like/heart count
	Comments  int64 `json:"comments"`  // Comment count
	Shares    int64 `json:"shares"`    // Share/repost count
	Bookmarks int64 `json:"bookmarks"` // Save/bookmark count
}

// Authentication represents auth status
type Authentication struct {
	Used   bool       `json:"used"`   // Whether authentication was used
	Source AuthSource `json:"source"` // Source of authentication
}

// ExtractResult represents a successful extraction result
type ExtractResult struct {
	URL            string         `json:"url"`            // Input URL
	Platform       string         `json:"platform"`       // Platform identifier
	MediaType      MediaType      `json:"mediaType"`      // Content classification
	Author         Author         `json:"author"`         // Creator information
	Content        Content        `json:"content"`        // Content metadata
	Engagement     Engagement     `json:"engagement"`     // Social metrics
	Media          []Media        `json:"media"`          // Media items
	Authentication Authentication `json:"authentication"` // Auth status
}

// Extractor is the interface that all platform extractors must implement
type Extractor interface {
	Match(url string) bool
	Extract(url string, opts ExtractOptions) (*ExtractResult, error)
}
