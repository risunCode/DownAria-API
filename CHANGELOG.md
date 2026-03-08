# CHANGELOG

All notable changes to **DownAria-API** are documented in this file.
This format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [1.3.0] - 2026-03-08

### Security
- Moved protected HLS gateway traffic fully behind signed `web` route handling instead of relying on the previous public-facing compatibility path.
- Removed permissive wildcard CORS behavior from HLS responses used by the signed web flow.

### Changed
- Removed unused legacy stream and duplicate HLS playlist rewriter code paths to lower maintenance overhead.
- Refactored the main HTTP handler construction into smaller setup helpers so infrastructure wiring is easier to follow and less brittle.
- Preserved public `v1` compatibility routes while tightening the intended `web/*` gateway path used by the frontend.
- Removed `/api/v1/hls-stream` and standardized backend HLS rewriting on the signed `/api/web/hls-stream` route.
- Removed the remaining backend `/api/web/hls-stream` transport route and its dedicated handler/test wiring because HLS streaming is no longer part of the active frontend flow.
- Removed legacy extraction error remapping and old request-id compatibility fallbacks so transport responses rely on the canonical error/request metadata path.
- Removed deprecated filename compatibility helpers and updated extractors to generate filenames through the single direct filename builder.

## [1.2.1] - 2026-03-06

### Changed
- Twitter native extractor now removes HLS (`.m3u8`) variants when progressive MP4-with-audio variants already exist for the same media item, while keeping HLS as fallback when no progressive audio-capable variant is available.
- Extraction response filename normalization now enforces unified naming per variant with sequence-aware index behavior.

### Fixed
- Prevented duplicate filename collisions in `variants[].filename` by using deterministic index suffix rules (`index 0` hidden, subsequent entries use `_<n>_`).
- Removed `unknown_*` style naming outcomes by improving author/title seed selection and Unicode-safe sanitization for mixed-language creator names/descriptions.
- Stopped embedding raw post IDs/timestamps in generated filenames for extractor output.

## [1.2.0] - 2026-03-05

### Added
- Structured logging with `log/slog` for JSON output and better observability
- Enhanced health check endpoint with dependency monitoring (FFmpeg availability, memory pressure detection)
- Health status types: `healthy`, `degraded`, and `unhealthy` with appropriate HTTP status codes
- Dependency status reporting in health check responses
- Cookie file support for YouTube authentication via yt-dlp
- Netscape cookie format parser for proper YouTube authentication handling
- Cookie parameter support in merge endpoint for authenticated video processing
- **HLS streaming support** with playlist and segment proxy capabilities (`/api/v1/hls-stream`, `/api/web/hls-stream`)
- **HLS merge support** for downloading and concatenating HLS segments into single files
- HLS playlist parser using `github.com/grafov/m3u8` library for master and media playlist handling
- HLS URL rewriter for proxying playlists and segments through DownAria infrastructure
- Concurrent segment downloader with configurable worker pool (5-10 workers) and retry logic
- Streaming downloader with buffer pool for memory-efficient large file transfers
- Pipeline infrastructure for chaining download and merge operations
- Optimized HTTP client with connection pooling and configurable timeouts
- Buffer pool for reusable byte buffers to reduce GC pressure
- HEAD request deduplicator using singleflight pattern to prevent duplicate metadata fetches
- Metrics system tracking downloads, streams, cache hits, active connections, and bandwidth
- Metrics endpoint (`/metrics`) exposing Prometheus-compatible statistics
- Feature flag middleware with percentage-based rollout support for gradual feature deployment
- FFmpeg merge infrastructure with progress tracking and error handling
- Comprehensive error code documentation (`Documentation/ERROR_CODES.md`)
- Integration tests for content delivery optimization flows
- Configuration options: `HLS_STREAMING_ENABLED`, `HLS_STREAMING_ROLLOUT`, `HLS_MERGE_ENABLED`, `HLS_SEGMENT_MAX_RETRIES`, `HLS_WORKER_POOL_SIZE`

### Changed
- Migrated all logging from `log.Printf` to structured `log/slog` with proper severity levels
- HTTP request logs now use JSON format with structured attributes for better log aggregation
- Health check endpoint now returns detailed system information including dependency status
- YouTube extraction now uses temporary cookie files instead of HTTP headers for authentication
- Proxy handler now uses streaming downloader for improved memory efficiency on large files
- Merge handler refactored to use new merge infrastructure with better error handling
- Network client architecture improved with separate optimized client for high-throughput scenarios
- Router updated with feature-gated HLS routes and metrics endpoint
- Documentation updated to reflect new HLS capabilities and content delivery optimizations

### Fixed
- **Critical:** HTTP request logs now show correct severity levels based on status code (info for 2xx/3xx, warn for 4xx, error for 5xx) instead of always showing "error"
- **Critical:** YouTube authentication now works correctly with user-provided cookies (previously failed with AUTH_REQUIRED error)
- **Critical:** HLS master playlist rewriter now correctly handles shared Alternative renditions (audio/subtitle tracks) to prevent recursive proxy URL generation
- Improved observability and monitoring capabilities with proper log severity classification
- Memory efficiency improved for large file downloads through streaming and buffer pooling
- Reduced duplicate HEAD requests through deduplication layer
- HLS alternative renditions (audio/subtitle tracks) are now rewritten only once even when shared across multiple video quality variants

## [1.1.1] - 2026-03-04

### Changed
- Origin middleware now fails closed when the allowlist is empty, and wildcard origin acceptance applies only when `*` is explicitly configured.
- Proxy auth resolver now accepts upstream auth from header-only input (`X-Upstream-Authorization`) and ignores query token auth.
- Synced integration/runtime docs to signed `/api/web/*` gateway behavior and current DownAria-API naming.
- Synced local integration defaults in env/docs references  

### Fixed
- Added compatibility guard tests covering `X-Downaria-*` signature headers and filename branding fallback behavior.

## [1.1.0] - 2026-03-03

### Added
- Unified URL sanitizer utility for strict HTTP/HTTPS canonicalization in outbound paths.
- Shared log-redaction utility for sensitive token/query/header masking in high-risk logs.
- MIME-first media classifier for Aria-Extended normalization (MIME -> extension -> codec fallback).
- Singleflight dedupe for extraction cache misses and proxy HEAD metadata fetches.

### Changed
- Extraction flow now supports generic fallback for non-native unknown URLs via Aria-Extended, while preserving native-first exception behavior.
- Proxy limits are now mode-aware: download path constrained to configured download cap, preview/proxy path supports larger safe ceiling.
- Filename generation strategy upgraded across extractors with safer sanitization and improved identifier composition.
- Route-level rate-limiting tightened with stricter controls for expensive merge paths.

### Security
- Applied request-body cap and `413` handling in web-signature middleware.
- Enforced merge exposure controls via config gating and protected route behavior.
- Added redirect-aware outbound SSRF guards and stricter outbound host validation.
- Strengthened command execution safety with bounded output capture and explicit timeout controls.

### Fixed
- Reduced false media-type detection by unifying variant/top-level classification logic.
- Fixed malformed filename edge cases (mojibake noise, missing `[DownAria]` closing bracket) in merge/download paths.
- Removed expensive full-stream size probing fallback in favor of bounded range probing.
- Added direct `videoUrl+audioUrl` merge-path support for non-YouTube stream pair merge requests.

## [1.0.0] - 2026-03-03

Initial unified production release of DownAria-API.

### Added
- Clean architecture layout across `internal/core`, `internal/app`, `internal/extractors`, `internal/infra`, and `internal/transport`.
- Unified extraction response envelope with stable top-level fields (`success`, `data`, `error`, `meta`).
- Native extractors for Facebook, Instagram, Threads, Twitter/X, TikTok, and Pixiv.
- Extended extractor path (yt-dlp wrapper) for YouTube.
- Public API routes: `/api/v1/extract`, `/api/v1/proxy`, `/api/v1/merge`, `/api/v1/stats/public`.
- Internal signed gateway routes: `/api/web/*` protected by `WEB_INTERNAL_SHARED_SECRET` signature validation.
- Native Threads extractor with HTML/data payload parsing, media URL extraction, and engagement parsing.
- Threads media dedupe and quality prioritization (video-first selection, noisy preview/profile variants filtered).
- Backend-driven filesize enrichment for variants including Threads (`HEAD`, range probe, stream fallback).
- Backend-driven download filename normalization via shared utility for proxy download responses.
- Stats persistence with buffered flush and atomic file writes.
- Configurable cache TTLs for extraction and proxy metadata.
- Retry framework for retryable extraction failures with exponential backoff.
- Rate-limit metadata and recovery hints (`Retry-After`, `retryAfter`, `resetAt`).

### Changed
- Consolidated historical `2.x` tracks into a single release line at `v1.0.0`.
- Project messaging aligned as an initial `v1.0.0` upgrade from the Next.js/Xtfetch era, adapted to the `Complete-half/Fetchtium_RE` extraction direction.
- Merge and streaming flow hardened for large payload handling and long-running operations.
- Proxy upstream header behavior refined for Threads/Instagram CDN compatibility.
- Frontend integration contract aligned to backend-owned metadata (filename/filesize source of truth).

### Fixed
- Prevented duplicate/low-value Threads media entries caused by CDN size variants and profile image noise.
- Fixed Threads thumbnail mapping so video entries use real image thumbnail sources instead of video URL placeholders.
- Resolved missing filesize for Threads variants in extraction responses.
- Improved response streaming error handling paths for merge/proxy stability.

---
