# Error Codes

This file lists stable API error codes used by `internal/core/errors/codes.go`.

## Content Delivery Optimization Codes

| Code | HTTP Status | Meaning |
|---|---:|---|
| `HLS_PLAYLIST_PARSE_FAILED` | 502 | Playlist fetch succeeded but parsing/rewrite pipeline failed. |
| `HLS_SEGMENT_FETCH_FAILED` | 502 | Segment streaming or segment download worker fetch failed. |
| `WORKER_POOL_FULL` | 503 | Merge worker queue is full; retry later. |

## Existing Common Codes

| Code | HTTP Status |
|---|---:|
| `INVALID_JSON` | 400 |
| `INVALID_URL` | 400 |
| `MISSING_PARAMS` | 400 |
| `ACCESS_DENIED` | 403 |
| `PROXY_FAILED` | 502 |
| `MERGE_FAILED` | 500 |
| `FILE_TOO_LARGE` | 413 |
