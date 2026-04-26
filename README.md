# DownAria-API

[![Version](https://img.shields.io/badge/version-2.2.0-blue)](./CHANGELOG.md)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](./LICENSE)

DownAria-API is a Go service for extracting media metadata and producing downloadable artifacts from supported social URLs through a unified JSON API.

## What It Does

- native-first extraction for owned platforms like Twitter/X
- universal extraction and source selection through `yt-dlp`
- synchronous `download`, `convert`, and `merge` endpoints, with `/download` as the primary smart artifact endpoint
- asynchronous jobs with persisted manifests and artifact fetch endpoints used as the follow-up path when `/download` returns `202`
- public API access without authentication
- structured error model, health reporting, and runtime safeguards
- pretty terminal logging mode with startup banner for interactive runs
- Unicode-safe filenames and attachment headers with ASCII fallback

## Current Internal Layout

```text
internal/
├── api/
├── auth/
├── config/
├── extract/
├── logging/
├── media/
├── outbound/
├── platform/
└── storage/
```

## Run

```bash
go run ./cmd/downaria-api
```

## Documentation

- API routes and async flow: `docs/api.md`
- Authentication: removed (all APIs are public)
- Error model and codes: `docs/errors.md`
- Environment, limits, health, and operations: `docs/config.md`
- Response and persistence schemas: `docs/models.md`
- End-to-end curl examples: `docs/examples.md`

## Key Runtime Notes

- native extractors always own their URLs first
- `yt-dlp` does not override native extractor ownership
- primary client flow is `extract -> download`; `/download` automatically escalates to merge, convert, or async job mode when needed
- when `/download` returns `202 Accepted`, clients should poll the returned job URLs instead of creating jobs manually first
- native media downloads use pooled Go HTTP via `grab`
- direct hot-link sources can carry `Referer`/`Origin`, are preflighted before full download, and may fall back to `yt-dlp` when stale
- request cookies are validated against real request hosts and never forwarded across host boundaries
- outbound protection is based on blocked/private network rejection plus redirect revalidation, not per-platform allowlists
- extraction cache uses shorter TTLs for signed or ephemeral source URLs
- merge outputs are validated with `ffprobe`
- merge requests include output-quality sanity checks against the requested target
- async artifacts are stored with TTL and quota cleanup
- `/health` caches temp-storage tree stats briefly so repeated health checks stay bounded

## Test

```bash
go test ./...
```
