# FetchMoona

High-performance Go-based API for extracting media from social platforms with a unified response format.

## Release

- **Current unified release:** `v1.1.0`
- **Lineage:** upgraded from the initial `v1.0.0` line with security/performance hardening and unified extraction improvements.

## Features

- **Unified Response Format** - Consistent structure across all platforms
- **Native Extractors** (Go): Facebook, Instagram, Threads, Twitter/X, TikTok, Pixiv
- **Extended Extractor** (yt-dlp): YouTube
- **Clean Architecture** - Layered architecture with clear boundaries
- **Caching Layer** - TTL cache for extraction results and proxy metadata
- **Stats Persistence** - File-backed stats with atomic writes
- **Secure Web Gateway** - Internal signed `/api/web/*` routes via `WEB_INTERNAL_SHARED_SECRET`
- **Large File Stability** - Improved merge/proxy behavior for large responses and long-running transfers
- **Multi-mode Merge** - Supports YouTube URL fast-path, direct `videoUrl+audioUrl` merges, and audio-only conversion

## Code Quality & Reliability Improvements

- **Categorized Errors** - API errors now include stable `category` values (`VALIDATION`, `NETWORK`, `RATE_LIMIT`, `AUTH`, `NOT_FOUND`, `EXTRACTION_FAILED`)
- **Rate-limit Recovery Hints** - Rate-limit responses include `Retry-After` header and metadata with `retryAfter` + `resetAt`
- **Extraction Retries** - Retryable extraction failures (network/rate-limit) use exponential backoff with configurable attempts/delay
- **Simple Cache TTL** - Extraction cache uses a single global TTL from environment configuration
- **Buffered Stats Persistence** - Public stats writes are buffered by threshold/interval and flushed atomically to reduce I/O churn
- **Threads Native Extraction** - Dedicated native Threads extractor with HTML/data payload parsing, media dedupe, and better thumbnail selection
- **Backend-driven Download Metadata** - Filename and filesize handling standardized from backend output

## Quick Start

```bash
# Download dependencies
go mod download

# Run server
go run ./cmd/server

# Build binary
go build -o fetchmoona ./cmd/server
```

## API Endpoints

Active routes are registered in `internal/transport/http/router.go`.

- Public utility: `GET /`, `GET /health`, `GET /api/settings`, `GET /api/v1/stats/public`
- Web-protected: `POST /api/web/extract`, `GET /api/web/proxy`, `GET /api/web/download`, `POST /api/web/merge` (merge route also gated by `MERGE_ENABLED`)
- Public API: `POST /api/v1/extract`, `GET /api/v1/proxy`, `GET /api/v1/download`
- Conditional public merge: `POST /api/v1/merge` is only registered when `WEB_INTERNAL_SHARED_SECRET` is empty

Production posture:

- `cmd/server/main.go` requires `WEB_INTERNAL_SHARED_SECRET` and exits when it is missing
- Because of that, signed `/api/web/merge` is the primary production merge route

### POST /api/v1/extract

Extract media from a URL.

**Request:**
```json
{
  "url": "https://twitter.com/user/status/123456"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://twitter.com/user/status/123456",
    "platform": "twitter",
    "mediaType": "post",
    "author": {
      "name": "User Name",
      "handle": "username"
    },
    "content": {
      "id": "123456",
      "text": "Tweet content",
      "description": "Tweet content",
      "createdAt": "2026-03-01T10:00:00Z"
    },
    "engagement": {
      "views": 1000,
      "likes": 50,
      "comments": 10,
      "shares": 5,
      "bookmarks": 0
    },
    "media": [
      {
        "index": 0,
        "type": "video",
        "thumbnail": "https://...",
        "variants": [
          {
            "quality": "720p",
            "url": "https://...",
            "format": "mp4",
            "mime": "video/mp4",
            "requiresProxy": false,
            "requiresMerge": false
          }
        ]
      }
    ],
    "authentication": {
      "used": false,
      "source": "none"
    }
  }
}
```

## Supported Platforms

| Platform | Type | Status |
|----------|------|--------|
| Facebook | Native | ✅ Working |
| Instagram | Native | ✅ Working |
| Threads | Native | ✅ Working |
| Twitter/X | Native | ✅ Working |
| TikTok | Native | ✅ Working |
| Pixiv | Native | ✅ Working |
| YouTube | yt-dlp | ✅ Working |

## Docker

```bash
# Build image
docker build -t fetchmoona:latest .

# Run container
docker run --rm -p 8080:8080 --env-file .env fetchmoona:latest
```

Healthcheck endpoint: `GET /health`

## Railway

This repository includes a `railway.toml` configured for Dockerfile deployment.

```bash
# Optional local check
railway up
```

Set required variables in Railway:

- `WEB_INTERNAL_SHARED_SECRET`
- `ALLOWED_ORIGINS`
- `PUBLIC_BASE_URL`
- `MAX_DOWNLOAD_SIZE_MB` (optional override)

## Architecture

```
/internal/
├── app/services/extraction/    # Application services
├── core/                       # Domain contracts
│   ├── config/                 # Configuration
│   ├── errors/                 # Error codes
│   └── ports/                  # Interfaces (StatsStore)
├── extractors/                 # Platform extractors
│   ├── core/                   # Extractor types
│   ├── native/                 # Native Go extractors
│   ├── aria-extended/          # yt-dlp wrapper
│   └── registry/               # Registry pattern
├── infra/                      # Infrastructure
│   ├── cache/                  # TTL cache
│   ├── network/                # HTTP client
│   ├── persistence/            # Stats storage
│   └── profiling/              # pprof server
├── shared/util/                # Utilities
└── transport/http/             # HTTP transport
    ├── handlers/
    ├── middleware/
    └── router.go

/pkg/
├── ffmpeg/                     # FFmpeg wrapper
├── media/                      # MIME utilities
└── response/                   # Response builders
```

## Project Structure

| Layer | Path | Responsibility |
|-------|------|----------------|
| Core | `internal/core/` | Contracts, interfaces, config |
| Extractors | `internal/extractors/` | Platform extraction logic |
| Application | `internal/app/services/` | Use case orchestration |
| Infrastructure | `internal/infra/` | Technical implementations |
| Transport | `internal/transport/` | HTTP/gRPC handlers |
| Shared | `internal/shared/` | Utilities |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `ALLOWED_ORIGINS` | _(empty)_ | Comma-separated allowed origins |
| `TRUSTED_PROXY_CIDRS` | _(empty)_ | Comma-separated trusted proxy CIDRs/IPs used for client IP extraction |
| `UPSTREAM_TIMEOUT_MS` | `10000` | Upstream timeout in milliseconds |
| `GLOBAL_RATE_LIMIT_WINDOW` | `60/1m` | Global IP rate limit in `<limit>/<window>` format (supports `5m`, `5min`, `1h`) |
| `GLOBAL_RATE_LIMIT_MAX_BUCKETS` | `10000` | Max in-memory rate-limit buckets |
| `GLOBAL_RATE_LIMIT_BUCKET_TTL` | `10m` | TTL for idle rate-limit buckets |
| `MAX_DOWNLOAD_SIZE_MB` | `1024` | Max proxied file size in MB |
| `MAX_MERGE_OUTPUT_SIZE_MB` | `512` | Max merge/audio-conversion output size in MB |
| `SERVER_READ_TIMEOUT` | `15s` | HTTP server read timeout |
| `SERVER_READ_HEADER_TIMEOUT` | `10s` | HTTP server read-header timeout |
| `SERVER_WRITE_TIMEOUT` | `15m` | HTTP server write timeout |
| `SERVER_IDLE_TIMEOUT` | `60s` | HTTP server idle timeout |
| `SERVER_MAX_HEADER_BYTES` | `1048576` | HTTP max request header size |
| `WEB_INTERNAL_SHARED_SECRET` | _(required at runtime)_ | Required for server startup and `/api/web/*` request signature verification |
| `STATS_PERSIST_ENABLED` | `false` | Enable file-backed stats persistence |
| `STATS_PERSIST_FILE_PATH` | `./data/public_stats.json` | Stats file path |
| `STATS_PERSIST_FLUSH_INTERVAL_MS` | `5000` | Flush interval in ms |
| `STATS_PERSIST_FLUSH_THRESHOLD` | `10` | Flush after N buffered stat updates |
| `EXTRACTION_MAX_RETRIES` | `3` | Extraction attempts for retryable failures |
| `EXTRACTION_RETRY_DELAY_MS` | `500` | Base retry delay with exponential backoff |
| `CACHE_EXTRACTION_TTL` | `5m` | Global extraction cache TTL |
| `CACHE_PROXY_HEAD_TTL` | `45s` | Proxy HEAD cache TTL |

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package
go test ./internal/app/services/extraction/...
```

## Documentation

- [API Routes](Documentation/API_Routes.md) - Endpoint documentation
- [Architecture](Documentation/Architecture.md) - Architecture overview
- [Error Codes](Documentation/ERROR_CODES.md) - Error code, HTTP status, and category reference
- [Error Handling](Documentation/ERROR_HANDLING.md) - Error mapping, retries, and rate-limit metadata behavior
- [Configuration](Documentation/CONFIGURATION.md) - Environment variables and runtime configuration behavior
- [Deployment](Documentation/DEPLOYMENT.md) - Production deployment and operational guidance

## License

GPL-3
