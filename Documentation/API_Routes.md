# Endpoints and API Routes

This document is aligned with the active routes in `internal/transport/http/router.go`.

All JSON endpoints use a consistent response envelope:

```json
{
  "success": true,
  "data": {}
}
```

Or on error:

```json
{
  "success": false,
  "error": {
    "code": "...",
    "message": "..."
  }
}
```

Note: proxy and merge endpoints may return binary streams.

---

## 1) Health and public utility routes

### `GET /health`

- Simple JSON health check.
- Example fields: `status`, `timestamp`.

### `GET /api/settings`

- Returns public backend settings such as `public_base_url`, `merge_enabled`, `upstream_timeout_ms`, `allowed_origins`, and `max_download_size_mb`.

### `GET /api/v1/stats/public`

- Returns public runtime stats (visits, extractions, downloads).
- This endpoint also records a visitor hit per request.

---

## 2) Route groups: `/api/web/*` vs `/api/v1/*`

Core endpoints are exposed under two prefixes with the same handlers.
Primary frontend runtime flow targets signed `/api/web/*` routes.

### `/api/web/*` (frontend-protected)

- `POST /api/web/extract`
- `GET /api/web/proxy`
- `GET /api/web/download`
- `POST /api/web/merge`

Additional middleware on this group:

- Origin check (`RequireOrigin`)
- Anti-bot filter (`BlockBotAccess`)
- Web signature verification (`RequireWebSignature` using `X-Downaria-Timestamp`, `X-Downaria-Nonce`, `X-Downaria-Signature`)
- Merge route is additionally gated by `MERGE_ENABLED` (`RequireMergeEnabled`)

### `/api/v1/*` (public API)

- `POST /api/v1/extract`
- `GET /api/v1/proxy`
- `GET /api/v1/download`
- `POST /api/v1/merge` (registered only when `WEB_INTERNAL_SHARED_SECRET` is empty)

The `/api/v1/*` group does not apply origin-protection or anti-bot middleware.
`POST /api/v1/merge` should be treated as conditional compatibility mode, not the default production runtime path.

---

## 3) Core endpoint behavior

### `POST /api/web/extract` and `POST /api/v1/extract`

- Request JSON: `{ "url": "...", "cookie": "optional" }`
- Validates `http/https` URL, selects extractor through the registry, executes extraction, and records extraction metrics.
- Success response returns the extracted payload in `data` (no nested `result`).

### `GET /api/web/proxy` and `GET /api/v1/proxy`

- Required query: `url`
- Optional query: `head=1`, `download=1`
- Supports forwarding `Range` for stream/seek.
- `head=1` returns header metadata (`X-File-Size`, `Content-Type`) without a body.
- File size behavior is mode-aware:
  - Preview/proxy mode (`download=0`): capped at 10 GB (`maxProxyPreviewSizeMB`)
  - Download mode (`download=1`): capped by `MAX_DOWNLOAD_SIZE_MB`

### `GET /api/web/download` and `GET /api/v1/download`

- Same handler as proxy endpoint but forced download mode.
- Equivalent to proxy request with `download=1`.
- Applies `MAX_DOWNLOAD_SIZE_MB` cap and sets attachment-oriented `Content-Disposition`.

### `POST /api/web/merge` and `POST /api/v1/merge`

- Base request JSON fields: `url`, `videoUrl`, `audioUrl`, `quality`, `format`, `filename`, `userAgent`, `platform`.
- Supports three request modes:
  - YouTube URL fast-path (`url` with YouTube host): resolves separate streams via `yt-dlp`, then merges via FFmpeg.
  - Direct pair merge (`videoUrl` + `audioUrl` together): merges non-YouTube stream pairs directly via FFmpeg.
  - Audio-only conversion (`format=m4a|mp3` or matching `quality`): extracts audio stream and returns `m4a` or `mp3`.
- Validation rules:
  - `videoUrl` and `audioUrl` must be provided together.
  - If direct pair is not used, `url` is required.
  - Non-YouTube `url` is only accepted for audio-only conversion.
- Response is a binary attachment stream.
- Merge output size is capped by `MAX_MERGE_OUTPUT_SIZE_MB`.
- Response includes `Content-Disposition: attachment` with resolved output filename.
- Runtime default is signed `POST /api/web/merge`; public `POST /api/v1/merge` is conditional (only when `WEB_INTERNAL_SHARED_SECRET` is unset).

Examples:

```bash
# Video merge (fast-path, signed web route)
curl -X POST "http://localhost:8081/api/web/merge" \
  -H "X-Downaria-Timestamp: <unix-seconds>" \
  -H "X-Downaria-Nonce: <random-nonce>" \
  -H "X-Downaria-Signature: <hmac-signature>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","quality":"1080p","format":"mp4"}'

# Audio-only (M4A, signed web route)
curl -X POST "http://localhost:8081/api/web/merge" \
  -H "X-Downaria-Timestamp: <unix-seconds>" \
  -H "X-Downaria-Nonce: <random-nonce>" \
  -H "X-Downaria-Signature: <hmac-signature>" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","format":"m4a","filename":"track.m4a"}'

# Direct pair merge (non-YouTube streams, signed web route)
curl -X POST "http://localhost:8081/api/web/merge" \
  -H "X-Downaria-Timestamp: <unix-seconds>" \
  -H "X-Downaria-Nonce: <random-nonce>" \
  -H "X-Downaria-Signature: <hmac-signature>" \
  -H "Content-Type: application/json" \
  -d '{"videoUrl":"https://cdn.example.com/video.m3u8","audioUrl":"https://cdn.example.com/audio.m3u8","filename":"clip.mp4"}'
```

---

## 4) Inactive endpoints

- `GET /api/v1/status` is not present in the current router.
- Legacy documentation that references that status endpoint is no longer valid.

## 5) Route registration and production posture

- Router behavior is controlled by `internal/transport/http/router.go`.
- `POST /api/v1/merge` is intentionally not registered when `WEB_INTERNAL_SHARED_SECRET` is configured.
- `cmd/server/main.go` currently requires `WEB_INTERNAL_SHARED_SECRET`, so production deployments should use signed `/api/web/merge`.
