# FetchMoona Architecture

## Overview

FetchMoona is a backend service for social media URL parsing and media link extraction. The architecture follows Clean Architecture principles with clearly separated layers.

## Directory Structure

```text
internal/
├── app/
│   └── services/extraction/    # Application services (orchestration)
├── core/                       # Domain contracts (interfaces, config, errors)
│   ├── config/                 # Configuration types and loader
│   ├── errors/                 # Error codes and mapper
│   └── ports/                  # Interface definitions (StatsStore, etc.)
├── extractors/                 # Platform extractors (business logic)
│   ├── core/                   # Extractor types and interfaces
│   ├── native/                 # Native Go extractors
│   │   ├── facebook/
│   │   ├── instagram/
│   │   ├── tiktok/
│   │   ├── twitter/
│   │   └── pixiv/
│   ├── aria-extended/          # yt-dlp wrapper
│   └── registry/               # Extractor registry
├── infra/                      # Infrastructure implementations
│   ├── cache/                  # TTL cache and Redis stub
│   ├── network/                # HTTP client and streamer
│   ├── persistence/            # Stats persistence
│   └── profiling/              # pprof profiling server
├── shared/                     # Shared utilities
│   └── util/                   # Request, convert, ID utilities
└── transport/http/             # HTTP transport layer
    ├── handlers/               # HTTP handlers
    ├── middleware/             # HTTP middleware
    └── router.go               # Route definitions

pkg/
├── ffmpeg/                     # FFmpeg wrapper
├── media/                      # MIME type utilities
└── response/                   # HTTP response utilities
```

## Layer Responsibilities

### 1. Core Layer (`internal/core/`)

Purpose: define contracts and interfaces used across the application.

- `config/`: configuration types and loading logic
- `errors/`: application error codes and HTTP status mapping
- `ports/`: interface definitions for repositories and services

```go
// Example: StatsStore interface
type StatsStore interface {
    RecordVisitor(visitorKey string, now time.Time)
    RecordExtraction(now time.Time)
    RecordDownload(now time.Time)
    Snapshot(now time.Time) StatsSnapshot
}
```

### 2. Extractors Layer (`internal/extractors/`)

Purpose: platform-specific extraction logic (business rules).

- `core/`: extractor interface and result types
- `native/`: native Go extractors per platform
- `aria-extended/`: yt-dlp wrapper for extended support
- `registry/`: registry pattern for extractor lookup

### 3. Transport Layer (`internal/transport/http/`)

Purpose: handle HTTP-specific concerns such as routing, middleware, and request/response processing.

- `router.go`: route definitions with chi router
- `handlers/`: HTTP request handlers
- `middleware/`: cross-cutting concerns (CORS, rate limiting, bot detection)

### 4. Application Layer (`internal/app/services/`)

Purpose: orchestrate use cases and coordinate extractors with infrastructure.

- `extraction/service.go`: main extraction service
- `extraction/cached_service.go`: caching decorator for extraction

### 5. Infrastructure Layer (`internal/infra/`)

Purpose: implement technical details such as caching, HTTP clients, and persistence.

- `cache/`: in-memory TTL cache and Redis stub
- `network/`: HTTP client and streamer
- `persistence/`: file-based stats persistence
- `profiling/`: pprof profiling server

### 6. Shared Layer (`internal/shared/`)

Purpose: reusable utilities with no business logic.

- `util/`: HTTP parsing, numeric conversion, and ID generation

## Key Design Decisions

### Interface Segregation

The stats store uses an interface to support multiple implementations:

```go
// Core interface
ports.StatsStore

// Current implementation
persistence.PublicStatsStore

// Future implementations: RedisStatsStore, SQLStatsStore, etc.
```

### Caching Strategy

- Extraction results: default 5m TTL, keyed by URL plus auth hash
- Proxy HEAD metadata: 45s TTL, keyed by URL plus auth/UA hash
- Implementation: in-memory TTL cache with on-access expiration and bounded entry count

### Middleware Stack

```go
// Global middleware
- CORS
- RequestID
- StructuredLogging
- RateLimit (global)
- RouteRateLimit (stricter for POST /api/v1/merge and POST /api/web/merge)

// Protected routes (web API)
- RequireOrigin
- BlockBotAccess
- RequireWebSignature
- RequireMergeEnabled (only on /api/web/merge)

// Public routes (v1 API)
- No origin/anti-bot/web-signature middleware
- POST /api/v1/merge only when WEB_INTERNAL_SHARED_SECRET is empty
```

### Error Handling

All errors include:

- Error code (string constant)
- HTTP status code
- User-friendly message

```go
apperrors.CodeInvalidURL          -> 400 Bad Request
apperrors.CodeUnsupportedPlatform -> 400 Bad Request
apperrors.CodeRateLimited         -> 429 Too Many Requests
```

## Extension Points

### Add a New Platform Extractor

1. Create extractor in `internal/extractors/native/<platform>/`
2. Register it in `extractors.RegisterDefaultExtractors()`
3. Add URL patterns in `internal/extractors/registry/patterns.go` (if required)

### Add a New Stats Backend

1. Implement `ports.StatsStore`
2. Replace store wiring in `handlers.NewHandler()`

### Add a New Transport Protocol

1. Create a new transport package (for example `internal/transport/grpc/`)
2. Reuse existing application services
3. Keep core business logic unchanged

## Testing Strategy

### Unit Tests

- Extraction service logic
- Middleware functions
- Utility helpers

### Integration Tests

- Route handling
- Middleware chains
- Handler coordination

### End-to-End Tests

- Full extraction flow
- Proxy streaming
- Error scenarios

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `ALLOWED_ORIGINS` | _(empty)_ | CORS + `/api/web/*` origin allowlist |
| `TRUSTED_PROXY_CIDRS` | _(empty)_ | Trusted proxy CIDRs for client IP extraction |
| `UPSTREAM_TIMEOUT_MS` | `10000` | External request timeout (ms) |
| `MAX_DOWNLOAD_SIZE_MB` | `1024` | Download-mode proxy limit |
| `MAX_MERGE_OUTPUT_SIZE_MB` | `512` | Merge output stream size cap |
| `SERVER_READ_TIMEOUT` | `15s` | HTTP server read timeout |
| `SERVER_READ_HEADER_TIMEOUT` | `10s` | HTTP server read-header timeout |
| `SERVER_WRITE_TIMEOUT` | `15m` | HTTP server write timeout |
| `SERVER_IDLE_TIMEOUT` | `60s` | HTTP server idle timeout |
| `SERVER_MAX_HEADER_BYTES` | `1048576` | Max request header bytes |
| `GLOBAL_RATE_LIMIT_MAX_BUCKETS` | `10000` | Rate-limit bucket cap |
| `GLOBAL_RATE_LIMIT_BUCKET_TTL` | `10m` | Rate-limit bucket TTL |
| `STATS_PERSIST_ENABLED` | `false` | Persist stats to file |
| `STATS_PERSIST_FILE_PATH` | `./data/public_stats.json` | Stats file path |
| `STATS_PERSIST_FLUSH_INTERVAL_MS` | `5000` | Flush interval |
| `STATS_PERSIST_FLUSH_THRESHOLD` | `10` | Buffered flush threshold |
| `CACHE_EXTRACTION_TTL` | `5m` | Extraction result cache TTL |
| `CACHE_PROXY_HEAD_TTL` | `45s` | Proxy HEAD cache TTL |

## Future Roadmap

1. Performance: Redis cache and distributed rate limiting
2. Observability: metrics and distributed tracing
3. Protocols: gRPC endpoint
4. Storage: database-backed stats and extraction history
