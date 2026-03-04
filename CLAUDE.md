# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a monorepo containing **DownAria** - a media extraction and download platform with two main components:

- **DownAria2** (frontend): Next.js 16 app with TypeScript, Tailwind CSS 4
- **FetchMoona** (backend): Go 1.24 API with clean architecture

**Supported Platforms:**
- YouTube (via yt-dlp wrapper)
- Instagram (native Go extractor)
- Twitter/X (native Go extractor)
- TikTok (native Go extractor)
- Facebook (native Go extractor)
- Pixiv (native Go extractor)
- Threads (native Go extractor)

**Key Features:**
- Multi-platform media extraction with unified response format
- Preview/stream and download flows with proxy support
- Video+audio merge capability (requires FFmpeg)
- HLS playlist rewriting for streaming
- Rate limiting with retry metadata
- Extraction result caching with configurable TTL
- Optional file-backed stats persistence

## Development Commands

### Running Both Services

```bash
# Windows: Start both frontend and backend in separate windows
dev.cmd

# Manual start (from root):
# Terminal 1 - Backend (with live reload)
cd FetchMoona && go run github.com/air-verse/air@latest -c .air.toml

# Terminal 2 - Frontend
cd DownAria2 && npm run dev
```

**Service URLs:**
- Frontend: http://localhost:3001
- Backend: http://localhost:8081 (dev) or :8080 (default)

### DownAria2 (Frontend)

```bash
cd DownAria2

# Development
npm run dev              # Start dev server on port 3001

# Building
npm run build            # Runs prebuild tasks, then builds
npm run start            # Start production server

# Quality
npm run lint             # Run ESLint
npm run test             # Run Vitest tests

# Prebuild tasks (automatic on build):
# - Updates service worker timestamp
# - Copies root CHANGELOG.md to public/Changelog.md
```

### FetchMoona (Backend)

```bash
cd FetchMoona

# Development
go run ./cmd/server                                    # Run server directly
go run github.com/air-verse/air@latest -c .air.toml   # Run with live reload

# Building
go build -o fetchmoona ./cmd/server    # Build binary

# Testing
go test ./...                          # Run all tests
go test ./... -v                       # Verbose output
go test ./internal/app/services/...    # Test specific package
```

## Architecture

### Frontend-Backend Integration (BFF Pattern)

The frontend uses a **Backend-for-Frontend (BFF)** pattern with signed gateway routes:

1. **Frontend runtime** calls local `/api/web/*` routes (DownAria2)
2. **BFF routes** sign requests with `WEB_INTERNAL_SHARED_SECRET` and forward to backend
3. **Backend** validates signatures and processes requests

**Key routes:**
- `POST /api/web/extract` - Extract media from URL
- `GET /api/web/proxy` - Stream/preview media (includes HLS playlist rewriting)
- `GET /api/web/download` - Download media files
- `POST /api/web/merge` - Merge video+audio streams

**Backend also exposes public `/api/v1/*` routes** for direct integrations (same endpoints without signature requirement).

**Critical:** `WEB_INTERNAL_SHARED_SECRET` must match between frontend and backend. Backend will not start without it.

### DownAria2 Structure

```
src/
├── app/                    # Next.js App Router pages and API routes
│   ├── api/
│   │   ├── web/           # BFF gateway routes (signed forwarding)
│   │   ├── feedback/      # Feedback webhook handler
│   │   └── stats/         # Stats proxy
│   ├── [locale]/          # i18n route groups
│   └── page.tsx           # Home page
├── components/            # React components
│   ├── core/             # Core UI components
│   ├── layout/           # Layout components
│   ├── media/            # Media player/preview components
│   └── ui/               # Reusable UI primitives
├── hooks/                # Custom React hooks
├── lib/                  # Utilities and shared logic
│   ├── api/             # API client and schemas
│   ├── errors/          # Error handling
│   └── storage/         # IndexedDB and localStorage wrappers
├── types/               # TypeScript type definitions
└── i18n/                # Internationalization config
```

**Path alias:** Use `@/*` to import from `src/*` (configured in tsconfig.json and vitest.config.ts)

**BFF Gateway Pattern:**
- Frontend makes requests to local `/api/web/*` routes
- These routes sign requests with HMAC signatures (see `src/app/api/web/_internal/signature.ts`)
- Signed requests are forwarded to backend API
- Signature includes: method, path, timestamp, nonce, body hash

### Security Architecture

**Request Signing:**
- Frontend generates HMAC-SHA256 signatures for `/api/web/*` requests
- Canonical string format: `METHOD\nPATH\nTIMESTAMP\nNONCE\nBODY_HASH`
- Headers: `X-Downaria-Timestamp`, `X-Downaria-Nonce`, `X-Downaria-Signature`
- Backend validates signatures in `internal/transport/http/middleware/web_signature.go`

**Security Headers:**
- CSP configured in `next.config.ts` with strict directives
- X-Frame-Options, X-Content-Type-Options, HSTS, etc.

**Rate Limiting:**
- Global IP-based rate limiting with configurable window/limit
- Separate stricter limits for merge routes (1/3 of global limit)
- Trusted proxy CIDR support for accurate client IP extraction

### FetchMoona Structure

Clean architecture with clear layer boundaries:

```
internal/
├── core/                      # Domain contracts and configuration
│   ├── config/               # Environment config loader
│   ├── errors/               # Error codes and mapping
│   └── ports/                # Interfaces (e.g., StatsStore)
├── extractors/               # Platform extraction logic
│   ├── core/                # Shared extractor types and utilities
│   ├── native/              # Native Go extractors (FB, IG, Twitter, TikTok, Pixiv)
│   ├── aria-extended/       # yt-dlp wrapper for YouTube
│   └── registry/            # Extractor registry and URL pattern matching
├── app/services/            # Application use cases
│   └── extraction/          # Extraction orchestration with retries
├── infra/                   # Infrastructure implementations
│   ├── cache/              # TTL cache for extraction results
│   ├── network/            # HTTP client
│   ├── persistence/        # File-backed stats storage
│   └── profiling/          # pprof server
└── transport/http/          # HTTP layer
    ├── handlers/           # Route handlers
    ├── middleware/         # CORS, rate limiting, signature validation
    └── router.go           # Route registration

pkg/                         # Public packages
├── ffmpeg/                 # FFmpeg wrapper for merging
├── media/                  # MIME type utilities
└── response/               # Response builders
```

**Key patterns:**
- **Extractor Registry**: URL patterns are matched against registered extractors using regex. Each extractor implements the `core.Extractor` interface.
- **Middleware Chain**: Applied in reverse order in `internal/app/app.go` - CORS, RequestID, StructuredLogging, RateLimit, RouteRateLimit
- **Signed Gateway Routes**: `/api/web/*` routes require HMAC signature verification using `WEB_INTERNAL_SHARED_SECRET`
- **Dual Route System**:
  - `/api/web/*` - Protected routes for frontend (requires signature)
  - `/api/v1/*` - Public API routes
  - `/api/v1/merge` only registered when `WEB_INTERNAL_SHARED_SECRET` is empty (production uses signed `/api/web/merge`)
- Errors use categorized codes (`VALIDATION`, `NETWORK`, `RATE_LIMIT`, `AUTH`, `NOT_FOUND`, `EXTRACTION_FAILED`)
- Rate-limit responses include `Retry-After` header and reset metadata
- Extraction failures use exponential backoff retries for transient errors

**Entry Point:** `cmd/server/main.go` loads environment variables from `.env.local` and `.env`, validates `WEB_INTERNAL_SHARED_SECRET`, and starts the application.

## Environment Configuration

Both services require `.env.local` or `.env` files. See `.env.example` in each directory.

**Critical shared variables:**
- `WEB_INTERNAL_SHARED_SECRET` - Must match between frontend and backend (required for backend startup)
- `NEXT_PUBLIC_API_URL` - Frontend's backend API URL (e.g., `http://localhost:8081`)
- `ALLOWED_ORIGINS` - Backend CORS origins (e.g., `http://localhost:3001`)

**Frontend-specific:**
- `NEXT_PUBLIC_APP_URL` - Frontend origin for signature context
- `NEXT_PUBLIC_BASE_URL` - Canonical public URL for metadata
- `FEEDBACK_DISCORD_WEBHOOK_URL` - Discord webhook for feedback API

**Backend-specific:**
- `PORT` - Server port (default: 8080, dev: 8081)
- `PUBLIC_BASE_URL` - Backend public URL
- `UPSTREAM_TIMEOUT_MS` - Timeout for upstream requests (default: 10000)
- `GLOBAL_RATE_LIMIT_WINDOW` - Rate limit config (e.g., `60/1m`)
- `MAX_DOWNLOAD_SIZE_MB` - Max proxied file size (default: 1024)
- `STATS_PERSIST_ENABLED` - Enable file-backed stats (default: false)
- `EXTRACTION_MAX_RETRIES` - Retry attempts for transient failures (default: 3)
- `CACHE_EXTRACTION_TTL` - Extraction cache TTL (default: 5m)

## Testing

**Frontend:**
- Uses Vitest with jsdom environment
- Test files: `*.test.ts`, `*.test.tsx` (169 test files)
- Run: `npm run test` in DownAria2 directory

**Backend:**
- Uses Go's built-in testing framework
- Test files: `*_test.go` (28 test files)
- Run: `go test ./...` in FetchMoona directory
- Integration tests in `internal/extractors/extractor_integration_test.go`

## Important Conventions

**Error Handling:**
- Backend uses categorized errors with stable error codes (see `internal/core/errors/codes.go`)
- Categories: `VALIDATION`, `NETWORK`, `RATE_LIMIT`, `AUTH`, `NOT_FOUND`, `EXTRACTION_FAILED`
- Rate-limit responses include `Retry-After` header and metadata with `retryAfter` + `resetAt`
- Frontend maps backend errors to user-friendly messages (see `src/lib/errors/`)

**Caching:**
- Backend: TTL-based cache for extraction results and proxy metadata
- Frontend: IndexedDB for history, settings, and content cache
- Extraction cache TTL controlled by `CACHE_EXTRACTION_TTL` environment variable

**Stats Persistence:**
- Backend can persist public stats to file (`STATS_PERSIST_ENABLED=true`)
- Buffered writes with configurable flush interval and threshold
- File path: `./data/public_stats.json` (default)

**Prebuild Scripts:**
- `scripts/update-sw-version.js` - Updates service worker build timestamp
- `scripts/copy-changelog.js` - Copies root `CHANGELOG.md` to `public/Changelog.md`

## Working with Extractors

When adding or modifying platform extractors:

1. Implement the `core.Extractor` interface in `internal/extractors/native/<platform>/`
2. Register URL patterns in `internal/extractors/registry/patterns.go`
3. Add factory function to registry in extractor initialization
4. Return unified response format (see `internal/extractors/core/types.go`)
5. Handle authentication if required (cookies, tokens)
6. Add tests in `<platform>/extractor_test.go`

## Important Notes

### Frontend
- Service worker timestamp is auto-updated during prebuild
- CHANGELOG.md is copied to public/ during prebuild for docs page
- Uses IndexedDB for history/cache, localStorage for settings
- i18n with next-intl (locale routing)
- PWA-enabled with manifest and service worker

### Backend
- **Requires `WEB_INTERNAL_SHARED_SECRET` to start** - will exit if missing
- Uses godotenv to load `.env.local` and `.env` automatically
- Graceful shutdown with 10-second timeout
- Supports both native extractors (Go) and extended extractor (yt-dlp)
- Rate limiting is IP-based with configurable buckets and TTL
- Stats persistence is optional and uses atomic file writes

### Merge Functionality
- Supports two modes: YouTube URL fast-path and direct `videoUrl+audioUrl` pair
- Frontend uses direct pair mode for HLS and split streams
- Requires FFmpeg binary available in PATH
- Output size limited by `MAX_MERGE_OUTPUT_SIZE_MB`

## Deployment

### Docker (Backend)

```bash
cd FetchMoona

# Build image
docker build -t fetchmoona:latest .

# Run container
docker run --rm -p 8080:8080 --env-file .env fetchmoona:latest
```

**Requirements:**
- FFmpeg must be available in container PATH for merge functionality
- Set `WEB_INTERNAL_SHARED_SECRET` in environment
- Configure `ALLOWED_ORIGINS` for CORS
- Health check endpoint: `GET /health`

### Railway

Both projects include deployment configurations:
- **FetchMoona**: `railway.toml` for Dockerfile deployment
- **DownAria2**: Standard Next.js deployment

See `FetchMoona/Documentation/DEPLOYMENT.md` for production environment recommendations.

### Vercel (Frontend)

- Vercel deployment configured via `vercel.json`
- PWA support with service worker in `public/sw.js`

## Dependencies

**Backend (FetchMoona):**
- Go 1.24+
- **FFmpeg** (required for `/api/web/merge` and `/api/v1/merge` routes)
- yt-dlp (for YouTube extraction)

**Frontend (DownAria2):**
- Node.js (compatible with Next.js 16)
- npm

## Documentation

Additional documentation in each project:
- **DownAria2/Documentation/**: API_Routes.md, Architecture.md, Env_Variables.md
- **FetchMoona/Documentation/**: API_Routes.md, Architecture.md, ERROR_CODES.md, ERROR_HANDLING.md, CONFIGURATION.md, DEPLOYMENT.md

## Common Pitfalls

- **Missing WEB_INTERNAL_SHARED_SECRET**: Backend will exit immediately if this is not set
- **CORS Issues**: Ensure `ALLOWED_ORIGINS` includes your frontend URL
- **Port Conflicts**: Frontend uses 3001, backend uses 8081 by default
- **Path Aliases**: Use `@/` prefix for imports in frontend (not relative paths)
- **Rate Limiting**: Merge routes have stricter limits (1/3 of global limit)
- **Signature Timing**: Frontend signatures include timestamp - ensure system clocks are synchronized
