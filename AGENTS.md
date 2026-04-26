# DownAria-API Agent Guide

This file is for coding agents working in this repository. (v2.2.0)
It captures the project's build/test commands, API intent, and coding style.

## Project Summary

- DownAria-API is a Go service for extracting media metadata and producing downloadable artifacts.
- Primary client flow is `extract -> download`.
- `/api/v1/download` is the main artifact endpoint.
- `/api/v1/download` may return either:
  - `200` with a streamed file, or
  - `202` with async job metadata to poll.
- Smart Selection (YouTube): Prefer direct non-HLS candidates first. Automatically fall back to alternate/HLS sources if primary choice fails preflight or download.
- `/api/v1/jobs` is a support endpoint for polling/artifact retrieval, not the normal first step for clients.
- Async jobs can be cancelled with `DELETE /api/v1/jobs/{id}`.
- Liveness/readiness probes are available at `/healthz/live` and `/healthz/ready`.

## Repo Layout

- `cmd/downaria-api/` - process entrypoint and wiring.
- `internal/api/` - HTTP handlers, envelopes, validation, health, async job routing.
- `internal/extract/` - shared extract pipeline, normalization, filename generation, cache.
- `internal/media/` - download, select, merge, convert orchestration.
- `internal/outbound/` - SSRF guard and outbound HTTP client behavior.
- `internal/platform/` - native and generic extractor integrations.
- `internal/storage/` - jobs and artifact persistence.
- `internal/runtime/` - temp root helpers, persistent stats path helpers, and process environment helpers.
- `docs/` - API, models, examples, errors, config, authentication.

## Runtime And Persistence

- Ephemeral task data belongs under `runtime.Root()` / `runtime.Subdir(...)`, which defaults to `os.TempDir()/downaria-api`.
  - This includes artifacts, jobs, downloads, merges, conversions, workspaces, extract cache unless explicitly configured, and temporary cookie files.
  - Override with `DOWNARIA_API_TEMP_DIR`.
- Stats are persistent local state and must not use the temp/runtime root.
  - `newStatsStore()` uses `runtime.EnsureStatsDir()`.
  - Default path is user local app data: `<local-app-data>/DownAria-API/stats/stats.json`.
  - Override with `DOWNARIA_API_STATS_DIR`.
- Do not reintroduce `DOWNARIA_API_RUNTIME_DIR`; only `DOWNARIA_API_TEMP_DIR` and `DOWNARIA_API_STATS_DIR` are supported.
- Keep `.env.example` minimal: required active values first, optional knobs commented out.

## External Rule Files

- No `.cursor/rules/`, `.cursorrules`, or `.github/copilot-instructions.md` files are present in this repo at the time of writing.
- If one is added later, treat it as higher priority than this file.

## Build Commands

- Run the service:
  - `go run ./cmd/downaria-api`
- Build the binary:
  - `go build -o downaria-api.exe ./cmd/downaria-api`
- Build without specifying output name:
  - `go build ./cmd/downaria-api`

## Test Commands

- Run all tests:
  - `go test ./...`
- Run one package:
  - `go test ./internal/api`
  - `go test ./internal/media`
- Run one named test in one package:
  - `go test ./internal/api -run TestDownloadEndpointUsesConvertForHLSAudioSource`
  - `go test ./internal/media -run TestSelectPrefersSeparateMP4AndM4A`
- Run tests verbosely:
  - `go test -v ./...`
- Re-run a package without test cache:
  - `go test -count=1 ./internal/api`

## Formatting And Tooling

- Format Go files with:
  - `gofmt -w <files>`
- The codebase relies on standard Go formatting rather than custom formatting rules.
- There is no dedicated lint configuration checked in right now; use idiomatic Go and keep code `gofmt`-clean.

## Verified Command Notes

- Full backend verification:
  - `go test ./...`
  - `go vet ./...`
- Focused runtime/stats verification:
  - `go test ./internal/runtime ./internal/api -count=1`

## Core Product Rules

- Preserve native-first extraction ownership.
  - Native extractors win for owned platforms.
  - Generic `yt-dlp` is fallback/universal, not the first choice when a native extractor applies.
- Keep source-page orchestration on the shared extract pipeline.
  - `/extract`, `/download`, `/merge`, and async download jobs should not drift into separate extraction behavior.
- Keep `/download` as the main artifact doorway.
  - It may resolve to direct download, merge, convert, or async follow-up.
- Keep `/jobs` as secondary support.
- Keep ephemeral files under the temp runtime root and persistent stats under the dedicated stats dir.
- Keep download/merge/convert source-page workflows on the shared extract pipeline.

## API Contract Rules

- Public JSON envelopes should stay consistent:
  - success: `success`, `response_time_ms`, `data`
  - error: `success`, `response_time_ms`, `error`
- Preserve public fields unless there is a deliberate contract change:
  - `extract_profile`
  - `filename`
  - stable error `kind` / `code`
- `X-DownAria-API-Mode` is diagnostic metadata, not the main public contract.
- If `/download` returns `202`, clients are expected to follow returned job URLs.
- If clients cancel jobs, cancellation should propagate through `JobManager.Cancel` and mark state as `cancelled`.
- Request-scoped cookies are accepted for any valid request host by default.
- Cookie headers require at least one valid destination URL; reject cookie-only requests without a valid destination.
- `/api/v1/download`, `/api/v1/merge`, and `/api/v1/convert` are media routes and should stay wrapped by rate limiting when a limiter is configured.

## Authentication

**Authentication has been completely removed from DownAria-API.**

All API endpoints are now public and do not require any authentication:
- `/api/v1/*` endpoints are publicly accessible
- No authentication layer is configured
- No session management is required
- All requests are processed without authentication checks

### Configuration
- Media rate limiting: `MEDIA_RATE_LIMIT`, `MEDIA_RATE_BURST`
- Runtime paths: `DOWNARIA_API_TEMP_DIR`, `DOWNARIA_API_STATS_DIR`
- Limits: `MAX_DOWNLOAD_BYTES` (default 892MB), `MAX_OUTPUT_BYTES` (default 892MB)
- Media tuning: `WORKSPACE_TTL`, `YTDLP_CONCURRENT_FRAGMENTS`

### Standalone Mode
- All `/api/v1/*` endpoints are available without authentication
- No auth configuration is required or processed
- The service operates in public API mode by default

## Health And Diagnostics

- `/health` is the richer diagnostic endpoint and can render HTML or JSON.
- When auth is configured, unauthenticated `/health` should expose only minimal safe data.
- `/healthz/live` always returns basic liveness.
- `/healthz/ready` reflects readiness via `RouterOptions.ReadyFn`.
- File transfer stability: response write deadlines are disabled for `/proxy`, `/download` (stream), and job artifact delivery.
- Expensive health values are cached:
  - process memory: 10 seconds
  - disk/RAM/network: 5 seconds
- Health dashboard HTML lives in `internal/api/health_dashboard.html` and is embedded with `go:embed`; do not move it back into a raw string.

## Code Style

- Follow idiomatic Go.
- Prefer small, ownership-based packages under `internal/`.
- Reuse existing helpers before adding new abstractions.
- Avoid duplicating orchestration across sync and async paths.
- Prefer explicit control flow over clever abstractions.
- Keep exported APIs minimal.
- Keep structs and field names clear and stable.
- Use `camelCase` for private identifiers and `PascalCase` for exported ones.
- Prefer descriptive names over shortened ones unless the abbreviation is standard Go (`ctx`, `req`, `err`).

## Imports

- Use standard Go import grouping as produced by `gofmt`.
- Keep standard library imports first, then internal module imports.
- Do not manually align or sort imports beyond what `gofmt` does.

## Types And Data Modeling

- Prefer concrete structs for API/data models.
- Use interfaces for service boundaries and pluggable behaviors only where already consistent with the package.
- Keep request/response structs explicit and JSON-tagged.
- Preserve stable JSON field names and avoid renaming public fields casually.
- Use explicit config structs in `internal/config`; validate new env knobs in `validate.go`.

## Error Handling

- Prefer the structured app error model from `internal/extract`.
- Use stable `kind`/`code` pairs.
- Return safe client-facing messages.
- Preserve retryability semantics where applicable.
- Do not introduce ad-hoc error strings for public HTTP behavior when an existing code/kind fits.
- If you add a new public error code, update docs and tests.

## Correctness Priorities

- Prefer correctness over site-specific shortcuts.
- Avoid hardcoding platform names if a source-trait heuristic can solve the problem.
- Treat signed and hot URLs as ephemeral.
- Respect SSRF guard and outbound validation.
- Preflight direct media where appropriate.
- Propagate `Referer` and `Origin` when anti-hotlink behavior requires them.
- Do not leak cookies across host boundaries.
- Do not reintroduce cookie platform or host allowlists unless product requirements explicitly change.
- Do not reintroduce platform-specific outbound host allowlists unless product requirements explicitly change.
- Defer `ffmpeg`/merge requirements until the selected mode actually needs them.
- Validate merged/converted outputs with `ffprobe` where applicable.
- Preserve cancellation-aware contexts in async jobs, download, merge, and convert flows.
- Preserve job JSON read/write safety; job state files are synchronized by `JobManager`.
- Preserve ArtifactStore's narrow lock scope: heavy file moves happen before manifest/quota locks.

## Filenames And Unicode

- Preserve Unicode scripts in metadata and safe filenames.
- Keep filename sanitization deterministic.
- Preserve Windows reserved-name protections.
- Do not regress ASCII fallback behavior for `Content-Disposition`.

## Testing Expectations

- Add or update tests whenever touching:
  - extraction
  - selection
  - download
  - merge
  - convert
  - async orchestration
  - filename behavior
  - error taxonomy
- Prefer regression tests for real bugs already seen:
  - HLS handling
  - hot/signed URL expiry
  - selector mismatches
  - cookie/header propagation
  - async parity
  - Windows filename edge cases
- After substantial changes, run `go test ./...`.
- Run `go vet ./...` after broad backend/security/runtime changes.
- Add runtime path tests when touching `internal/runtime`.
- Add API tests when changing auth, health, stats, cancellation, or rate limiting.

## Docs Expectations

- Update docs when changing public API behavior, response fields, error codes, or primary flow guidance.
- Keep `docs/api.md`, `docs/errors.md`, `docs/models.md`, `docs/examples.md`, and `README.md` aligned.

## Do / Don't

- Do preserve native-first behavior.
- Do preserve smart `/download` behavior across sync and async flows.
- Do keep runtime/temp handling centralized.
- Do keep stats persistent and separate from temp runtime data.
- Don't bypass the shared extract pipeline for source-page workflows.
- Don't add platform-specific hacks when a generic heuristic can solve the issue.
- Don't introduce a new public field or error code without updating docs and tests.
- Don't reintroduce `DOWNARIA_API_RUNTIME_DIR`.
