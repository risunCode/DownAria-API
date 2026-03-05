package hls

import (
	"fmt"
	"net/url"

	"github.com/grafov/m3u8"
)

func RewriteMasterPlaylist(master *m3u8.MasterPlaylist, baseURL, routePrefix string) {
	// Track rewritten alternatives to avoid rewriting the same object multiple times
	// (m3u8 library shares Alternative pointers across variants)
	rewrittenAlts := make(map[*m3u8.Alternative]bool)

	for _, v := range master.Variants {
		if v == nil || v.URI == "" {
			continue
		}
		abs := ResolveURL(v.URI, baseURL)
		v.URI = fmt.Sprintf("%s?url=%s", routePrefix, url.QueryEscape(abs))

		// Rewrite alternative renditions (audio/subtitle tracks)
		for _, alt := range v.Alternatives {
			if alt == nil || alt.URI == "" {
				continue
			}

			// Skip if already rewritten (shared pointer across variants)
			if rewrittenAlts[alt] {
				continue
			}

			absAlt := ResolveURL(alt.URI, baseURL)
			alt.URI = fmt.Sprintf("%s?url=%s", routePrefix, url.QueryEscape(absAlt))

			// Mark as rewritten
			rewrittenAlts[alt] = true
		}
	}
}

func RewriteMediaPlaylist(media *m3u8.MediaPlaylist, baseURL, routePrefix string) {
	for _, s := range media.Segments {
		if s == nil || s.URI == "" {
			continue
		}
		abs := ResolveURL(s.URI, baseURL)
		s.URI = fmt.Sprintf("%s?url=%s&chunk=1", routePrefix, url.QueryEscape(abs))
	}
	if media.Key != nil && media.Key.URI != "" {
		abs := ResolveURL(media.Key.URI, baseURL)
		media.Key.URI = fmt.Sprintf("%s?url=%s&chunk=1", routePrefix, url.QueryEscape(abs))
	}
	if media.Map != nil && media.Map.URI != "" {
		abs := ResolveURL(media.Map.URI, baseURL)
		media.Map.URI = fmt.Sprintf("%s?url=%s&chunk=1", routePrefix, url.QueryEscape(abs))
	}
}
