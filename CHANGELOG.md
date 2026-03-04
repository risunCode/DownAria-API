# CHANGELOG

All notable changes to **DownAria-API** are documented in this file.
This format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
