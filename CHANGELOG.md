# CHANGELOG

All notable changes to **DownAria-API** are documented in this file.
This format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
DownAria-API `v1.0.0` is the baseline release line going forward.
