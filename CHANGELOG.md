# Changelog

All notable changes to DownAria-API will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Changed
- Split runtime paths so ephemeral artifacts/jobs/downloads/merges use the OS temp root while persistent stats use user local app data.
- Documented `DOWNARIA_API_TEMP_DIR` and `DOWNARIA_API_STATS_DIR` as the only runtime path overrides.

### Security
- Updated utility route behavior around `/api/v1/proxy` and `/api/v1/stats/log`.
- Added job cancellation, health diagnostics gating, media route rate limiting, and request cookie destination validation.

---

## [2.2.0] - 2026-04-25

### Added
- **Backend Media Proxy** (`/api/v1/proxy`)
  - Supports **Range requests** for smooth media seeking in browsers
  - SSRF protection via outbound guard
  - CORS support for direct browser access
- **Stream Profile Enum**: Consolidated media type detection into a unified `stream_profile` field (`audio_only`, `muxed_progressive`, etc.)

### Changed
- **Simplified JSON Schema** (Breaking Change)
  - Flattened extraction response by removing nested `source` and `metadata` objects
  - Consolidated all metadata and source information directly under the `data` object
- **Downloader Engine**: Replaced `axel` with **`grab/v3`** (Pure Go)
  - Full support for **resumable downloads**
  - Better network recovery and progress reporting
  - Removed `axel` dependency from Docker image

### Improved
- **YouTube Playback**: Explicitly marked format 18 (360p) and 22 (720p) as muxed progressive streams to allow direct playback without merging
- **Logging**: Refined download stages (`Grab Download Start`, `Grab Download Complete`) for better visibility in server logs

---

## [2.1.0] - 2026-04-11

### Added
- **Dual API Route Surface**
  - `/api/v1/*` and `/api/web/*` compatibility routes (historical)
- **Response Helpers**
  - Shared JSON response helpers and route wiring cleanup
- **Shared Response Helpers**
  - `internal/api/response.go` for consistent JSON responses
  - Eliminated code duplication between v1 and web routes
- **Build Tag Support**
  - Platform-specific code separation (`api_windows.go`, `api_unix.go`)
  - Clean Windows syscall handling
- **Version Flag**
  - `--version` flag to print version and exit
  - Version logged on server startup

### Changed
- **Code Organization**
  - Merged API routes from 4 files to 2 files (v1/routes.go + web/routes.go)
  - 50% reduction in file count while maintaining readability
- **Documentation**
  - Updated API and configuration references
  - Updated AGENTS.md with auth implementation details

### Improved
- Added smarter YouTube download behavior by preferring `yt-dlp` for YouTube/Googlevideo paths instead of wasting time on fragile direct saves first.
- Added fallback from direct media save failures into `yt-dlp` for unstable YouTube-like sources.
- Improved Instagram error taxonomy with stable platform-specific codes such as `instagram_auth_required`, `instagram_media_not_found`, and `instagram_rate_limited`.
- Changed runtime stats persistence away from temp storage so counters do not reset as easily on restarts.

### Fixed
- Windows build compatibility with proper syscall isolation
- Removed unused imports and duplicate code
- Fixed downloader failures where direct Googlevideo saves could stall or fail before a better downloader path was tried.
- Fixed spoofable bootstrap flow by requiring stricter server-side validation.

### Security
- Hardened request validation and session-related controls (historical).

### Technical Details
- **Test Coverage**: 13 tests (historical snapshot)
- **Build Status**: All tests passing
- **Breaking Changes**: None - fully backward compatible

### Notes
- Standalone mode available.

---

## [2.0.0] - Previous Release

Initial stable release with:
- Native-first extraction (Twitter/X, Instagram, Facebook, TikTok, Pixiv, Threads)
- Universal extraction via yt-dlp
- Smart download endpoint with sync/async support
- Merge and convert capabilities
- Async job system with artifact storage
- Health monitoring and dependency checks
- Unicode-safe filename handling
- SSRF protection and outbound validation
- Structured error model
- Pretty terminal logging
