# Configuration

Configuration is loaded from environment variables in `internal/core/config/loader.go`.

## Core server settings

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | Accepts raw port (`8080`), `:8080`, `host:port`, URL, or `tcp/8080`; normalized to numeric port. |
| `ALLOWED_ORIGINS` | _(empty)_ | Comma-separated list. Empty means wildcard behavior in CORS middleware. |
| `PUBLIC_BASE_URL` | `http://localhost:<PORT>` | Returned by `/api/settings`. |
| `UPSTREAM_TIMEOUT_MS` | `10000` | Upstream HTTP timeout in milliseconds. Minimum effective value is `1`. |
| `GLOBAL_RATE_LIMIT_WINDOW` | `60/1m` | Global IP rate limit in `<limit>/<window>` format. Supports Go duration literals plus friendly forms (`5min`, `1hour`). |
| `MAX_DOWNLOAD_SIZE_MB` | `1024` | Max allowed proxied file size based on `Content-Length`. |
| `MERGE_ENABLED` | `false` | Exposed in `/api/settings`. Merge route exists regardless of this flag. |

## Extraction retry settings

| Variable | Default | Notes |
|---|---|---|
| `EXTRACTION_MAX_RETRIES` | `3` | Total attempts including first try. Minimum effective value is `1`. |
| `EXTRACTION_RETRY_DELAY_MS` | `500` | Base delay for exponential backoff (`x2` each retry, capped at `30s`). |

## Cache settings

### Extraction result cache

| Variable | Default | Notes |
|---|---|---|
| `CACHE_EXTRACTION_TTL` | `5m` | Global extraction cache TTL used for all platforms. |

The extraction cache key includes URL and cookie hash, so authenticated and unauthenticated requests do not collide.

### Proxy HEAD metadata cache

| Variable | Default | Notes |
|---|---|---|
| `CACHE_PROXY_HEAD_TTL` | `45s` | Parsed into config, but current proxy handler uses fixed internal TTL of `45s`. |

### Cache maintenance

| Variable | Default | Notes |
|---|---|---|
| `CACHE_CLEANUP_INTERVAL` | `5m` | Loaded into config for cache cleanup scheduling; not currently wired to a cleanup loop. |

## Stats persistence and buffering

| Variable | Default | Notes |
|---|---|---|
| `STATS_PERSIST_ENABLED` | `false` | Enables persisted public stats. |
| `STATS_PERSIST_FILE_PATH` | `./data/public_stats.json` | Atomic write target path. |
| `STATS_PERSIST_FLUSH_INTERVAL_MS` | `5000` | Time-based flush interval (`>=1000ms`). |
| `STATS_PERSIST_FLUSH_THRESHOLD` | `10` | Flush after N buffered stat events (`>=1`). |

With persistence enabled, stat updates are buffered and flushed asynchronously by threshold or interval, plus one final flush on graceful shutdown.

## Example `.env`

```env
PORT=8080
ALLOWED_ORIGINS=http://127.0.0.1:3000,http://localhost:3000
PUBLIC_BASE_URL=https://api.example.com

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
