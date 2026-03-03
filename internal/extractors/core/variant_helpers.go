package core

// NewVideoVariant creates an MP4 video variant.
func NewVideoVariant(quality, url string) Variant {
	return NewVariant(quality, url).
		WithFormat("mp4").
		WithMime("video/mp4")
}

// NewImageVariant creates a JPEG image variant.
func NewImageVariant(quality, url string) Variant {
	return NewVariant(quality, url).
		WithFormat("jpg").
		WithMime("image/jpeg")
}

// NewImageProxyVariant creates a JPEG image variant that requires proxying.
func NewImageProxyVariant(quality, url string) Variant {
	return NewImageVariant(quality, url).
		WithProxy(true)
}
