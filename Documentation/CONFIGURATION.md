# Configuration

Configuration is loaded from `internal/core/config/loader.go`.

For local dev alignment with the current DownAria frontend runtime, use FE `http://localhost:3001` and BE `http://localhost:8081` in `.env` values.

## Runtime and security

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | Accepts `8080`, `:8080`, `host:port`, URL, or `tcp/8080`; normalized to numeric port. Local stack examples use `8081`. |
| `ALLOWED_ORIGINS` | _(empty)_ | Comma-separated origin allowlist used by CORS and `/api/web/*` origin middleware. In `/api/web/*` origin middleware, an empty allowlist fails closed (no origins allowed). Use `*` explicitly to allow all origins. Include `http://localhost:3001` for default frontend dev origin. |
| `TRUSTED_PROXY_CIDRS` | _(empty)_ | Comma-separated trusted proxy CIDRs/IPs for client IP resolution in rate limiting and stats. |
| `WEB_INTERNAL_SHARED_SECRET` | _(empty in loader)_ | Required by `cmd/server/main.go` at startup; used to verify signed `/api/web/*` requests. When set, `POST /api/v1/merge` is not registered. |
| `PUBLIC_BASE_URL` | `http://localhost:<PORT>` | Returned by `/api/settings`. |

## Rate limiting

| Variable | Default | Notes |
|---|---|---|
| `GLOBAL_RATE_LIMIT_WINDOW` | `60/1m` | Global IP rate limit in `<limit>/<window>` format; supports Go duration plus friendly suffixes (`min`, `hour`, `sec`). |
| `GLOBAL_RATE_LIMIT_MAX_BUCKETS` | `10000` | In-memory bucket cap. Values `<100` fall back to `10000`. |
| `GLOBAL_RATE_LIMIT_BUCKET_TTL` | `10m` | Idle bucket TTL before cleanup/eviction. |

## HTTP server and upstream timeouts

| Variable | Default | Notes |
|---|---|---|
| `UPSTREAM_TIMEOUT_MS` | `10000` | Outbound HTTP timeout. Values `<1` fall back to `10000`. |
| `SERVER_READ_TIMEOUT` | `15s` | Server read timeout. |
| `SERVER_READ_HEADER_TIMEOUT` | `10s` | Server read-header timeout. |
| `SERVER_WRITE_TIMEOUT` | `15m` | Server write timeout (loader default). |
| `SERVER_IDLE_TIMEOUT` | `60s` | Server idle timeout. |
| `SERVER_MAX_HEADER_BYTES` | `1048576` | Max request header bytes. Values `<1024` fall back to `1048576`. |

## Merge and transfer limits

| Variable | Default | Notes |
|---|---|---|
| `MERGE_ENABLED` | `false` | Merge handler gate. Route exists but returns access denied when disabled. |
| `MAX_DOWNLOAD_SIZE_MB` | `1024` | Max download-mode size for proxy/download handlers. Preview/proxy mode uses a larger internal ceiling. |
| `MAX_MERGE_OUTPUT_SIZE_MB` | `512` | Max output stream size for merge/audio-conversion responses. |

## Content delivery optimization flags

| Variable | Default | Notes |
|---|---|---|
| `STREAMING_DOWNLOAD_ENABLED` | `true` | Enables buffered concurrent upstream->client streaming path. |
| `CONCURRENT_MERGE_ENABLED` | `false` | Enables concurrent downloader + merge worker pool path for direct pair merges. |
| `HLS_STREAMING_ENABLED` | `true` | Enables HLS playlist/segment optimized route behavior. |
| `HLS_MERGE_ENABLED` | `true` | Enables HLS playlist segment concatenation flow for merge inputs. |
| `MERGE_WORKER_COUNT` | `3` | Worker pool concurrency for merge jobs. |
| `HLS_SEGMENT_WORKER_COUNT` | `5` | Worker pool concurrency for HLS segment fetching. |
| `HLS_SEGMENT_MAX_RETRIES` | `3` | Retry attempts for failed HLS segment fetches. |
| `BUFFER_SIZE_VIDEO` | `262144` | Preferred video stream buffer size in bytes (256 KB). |
| `BUFFER_SIZE_AUDIO` | `65536` | Preferred audio stream buffer size in bytes (64 KB). |

## Gradual rollout controls

| Variable | Default | Notes |
|---|---|---|
| `FEATURE_STREAMING_DOWNLOAD_ROLLOUT` | `100` | Percent rollout for streaming download feature gate. |
| `FEATURE_CONCURRENT_MERGE_ROLLOUT` | `100` | Percent rollout for concurrent merge gate. |
| `FEATURE_HLS_STREAMING_ROLLOUT` | `100` | Percent rollout for HLS streaming gate. |
| `FEATURE_HLS_MERGE_ROLLOUT` | `100` | Percent rollout for HLS merge gate. |

## Stats persistence and buffering

| Variable | Default | Notes |
|---|---|---|
| `STATS_PERSIST_ENABLED` | `false` | Enables persisted public stats. |
| `STATS_PERSIST_FILE_PATH` | `./data/public_stats.json` | Atomic write target path. |
| `STATS_PERSIST_FLUSH_INTERVAL_MS` | `5000` | Flush interval in milliseconds (`>=1000`). |
| `STATS_PERSIST_FLUSH_THRESHOLD` | `10` | Flush after N buffered stat events (`>=1`). |

## Extraction and cache behavior

| Variable | Default | Notes |
|---|---|---|
| `EXTRACTION_MAX_RETRIES` | `3` | Total extraction attempts; minimum effective value is `1`. |
| `EXTRACTION_RETRY_DELAY_MS` | `500` | Base retry delay in milliseconds. |
| `CACHE_EXTRACTION_TTL` | `5m` | Extraction cache TTL. |
| `CACHE_PROXY_HEAD_TTL` | `45s` | Proxy HEAD metadata cache TTL. |
| `CACHE_CLEANUP_INTERVAL` | `5m` | General cache cleanup interval config value. |

## Example `.env`

```env
PORT=8081
ALLOWED_ORIGINS=http://localhost:3001,http://127.0.0.1:3001
TRUSTED_PROXY_CIDRS=127.0.0.1/32,10.0.0.0/8
WEB_INTERNAL_SHARED_SECRET=replace-with-random-secret
PUBLIC_BASE_URL=http://localhost:8081

UPSTREAM_TIMEOUT_MS=15000
SERVER_READ_TIMEOUT=15s
SERVER_READ_HEADER_TIMEOUT=10s
SERVER_WRITE_TIMEOUT=15m
SERVER_IDLE_TIMEOUT=60s
SERVER_MAX_HEADER_BYTES=1048576

GLOBAL_RATE_LIMIT_WINDOW=200/4min
GLOBAL_RATE_LIMIT_MAX_BUCKETS=10000
GLOBAL_RATE_LIMIT_BUCKET_TTL=10m

MERGE_ENABLED=true
MAX_DOWNLOAD_SIZE_MB=1024
MAX_MERGE_OUTPUT_SIZE_MB=512

EXTRACTION_MAX_RETRIES=3
EXTRACTION_RETRY_DELAY_MS=500

CACHE_EXTRACTION_TTL=5m
CACHE_PROXY_HEAD_TTL=45s
CACHE_CLEANUP_INTERVAL=5m

STATS_PERSIST_ENABLED=true
STATS_PERSIST_FILE_PATH=./data/public_stats.json
STATS_PERSIST_FLUSH_INTERVAL_MS=5000
STATS_PERSIST_FLUSH_THRESHOLD=10

STREAMING_DOWNLOAD_ENABLED=true
CONCURRENT_MERGE_ENABLED=false
HLS_STREAMING_ENABLED=true
HLS_MERGE_ENABLED=true
MERGE_WORKER_COUNT=3
HLS_SEGMENT_WORKER_COUNT=5
HLS_SEGMENT_MAX_RETRIES=3
BUFFER_SIZE_VIDEO=262144
BUFFER_SIZE_AUDIO=65536

FEATURE_STREAMING_DOWNLOAD_ROLLOUT=100
FEATURE_CONCURRENT_MERGE_ROLLOUT=100
FEATURE_HLS_STREAMING_ROLLOUT=100
FEATURE_HLS_MERGE_ROLLOUT=100
```
