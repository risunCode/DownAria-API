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

### `/api/web/*` (frontend-protected)

- `POST /api/web/extract`
- `GET /api/web/proxy`
- `POST /api/web/merge`

Additional middleware on this group:

- Origin check (`RequireOrigin`)
- Anti-bot filter (`BlockBotAccess`)

### `/api/v1/*` (public API)

- `POST /api/v1/extract`
- `GET /api/v1/proxy`
- `POST /api/v1/merge`

The `/api/v1/*` group does not apply origin-protection or anti-bot middleware.

---

## 3) Core endpoint behavior

### `POST /api/web/extract` and `POST /api/v1/extract`

- Request JSON: `{ "url": "...", "cookie": "optional" }`
- Validates `http/https` URL, selects extractor through the registry, executes extraction, and records extraction metrics.
- Success response includes `platform` and `result`.

### `GET /api/web/proxy` and `GET /api/v1/proxy`

- Required query: `url`
- Optional query: `head=1`, `download=1`
- Supports forwarding `Range` for stream/seek.
- `head=1` returns header metadata (`X-File-Size`, `Content-Type`) without a body.
- Enforces file size limits using `MAX_DOWNLOAD_SIZE_MB`.

### `POST /api/web/merge` and `POST /api/v1/merge`

- Request JSON: `{ "url": "YOUTUBE_URL", "quality": "optional", "format": "optional", "filename": "optional", "userAgent": "optional" }`
- Uses strict fast-path resolution with `yt-dlp` and FFmpeg only (no manual `videoUrl`/`audioUrl` fallback path).
- Supports:
  - video merge output (`mp4` stream)
  - audio extraction output (`mp3` or `m4a` stream)
- Response includes `Content-Disposition: attachment` with resolved output filename.

Examples:

```bash
# Video merge (fast-path)
curl -X POST "http://localhost:8080/api/v1/merge" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","quality":"1080p","format":"mp4"}'

# Audio-only (M4A)
curl -X POST "http://localhost:8080/api/v1/merge" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","format":"m4a","filename":"track.m4a"}'
```

---

## 4) Inactive endpoints

- `GET /api/v1/status` is not present in the current router.
- Legacy documentation that references that status endpoint is no longer valid.
