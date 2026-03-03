# Deployment

This guide covers deployment of the current Go server implementation.

## Prerequisites

- Go 1.22+ (or compatible with `go.mod`)
- Linux/Windows host with outbound internet access to platform/CDN endpoints
- Optional: FFmpeg binary installed and available in `PATH` for `/api/v1/merge`

## Build and run

```bash
go mod download
go build -o downaria-api ./cmd/server
./downaria-api
```

Server starts on `:<PORT>` and performs graceful shutdown (`SIGINT`/`SIGTERM`) with a `10s` timeout.

## Docker deployment

Build image:

```bash
docker build -t downaria-api:latest .
```

Run container:

```bash
docker run --rm -p 8080:8080 --env-file .env downaria-api:latest
```

Notes:

- Container listens on `0.0.0.0:${PORT}` (default `8080`).
- `/health` is used for health checking.
- Include `WEB_INTERNAL_SHARED_SECRET` in runtime env for signed `/api/web/*` routes.

## Railway deployment

This repository includes `railway.toml` configured for Dockerfile builds.

Recommended Railway variables:

```env
PORT=8080
PUBLIC_BASE_URL=https://<your-railway-domain>
ALLOWED_ORIGINS=https://<your-frontend-domain>
WEB_INTERNAL_SHARED_SECRET=<strong-random-secret>

UPSTREAM_TIMEOUT_MS=15000
GLOBAL_RATE_LIMIT_WINDOW=200/4min
MAX_DOWNLOAD_SIZE_MB=1024

EXTRACTION_MAX_RETRIES=3
EXTRACTION_RETRY_DELAY_MS=500

CACHE_EXTRACTION_TTL=5m
CACHE_PROXY_HEAD_TTL=45s
```

## Recommended production environment

```env
PORT=8080
PUBLIC_BASE_URL=https://api.example.com
ALLOWED_ORIGINS=https://downaria.example.com

UPSTREAM_TIMEOUT_MS=15000
GLOBAL_RATE_LIMIT_WINDOW=200/4min
MAX_DOWNLOAD_SIZE_MB=1024

EXTRACTION_MAX_RETRIES=3
EXTRACTION_RETRY_DELAY_MS=500

CACHE_EXTRACTION_TTL=5m

STATS_PERSIST_ENABLED=true
STATS_PERSIST_FILE_PATH=./data/public_stats.json
STATS_PERSIST_FLUSH_INTERVAL_MS=5000
STATS_PERSIST_FLUSH_THRESHOLD=10
```

## Reverse proxy recommendations

- Forward `X-Request-ID` (or allow backend to generate one).
- Keep `Range` header for `/api/v1/proxy` so partial content works.
- Preserve `Retry-After`, `X-RateLimit-*`, and `Content-Range` headers.
- Set upstream timeout higher than `UPSTREAM_TIMEOUT_MS` to avoid proxy-side premature timeout.

## Persistent data

When `STATS_PERSIST_ENABLED=true`:

- Mount/writeable directory for `STATS_PERSIST_FILE_PATH`.
- Ensure process has create/write permissions.
- Do not place stats file on ephemeral filesystem if counters must survive restart.

## Health and smoke checks

- `GET /health` should return `{"success":true,"data":{"status":"ok"...}}`
- `GET /api/settings` should reflect expected runtime config.
- `POST /api/v1/extract` should return categorized error payloads on invalid input.
- `GET /api/v1/stats/public` should show counters updating over traffic.

## Operational notes

- Request logging includes `request_id`, HTTP method/path, status, and latency.
- Extraction retries use exponential backoff and only retry network/rate-limit categories.
- Rate-limit errors expose both headers and JSON metadata (`retryAfter`, `resetAt`).
- Stats persistence uses buffered asynchronous flush + atomic file replacement for durability.
