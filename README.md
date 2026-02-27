# BlobBus

BlobBus is a minimal container-to-container blob exchange service.

## Features

- `POST /v1/blobs` upload raw bytes with required `Content-Length`
- `GET /v1/blobs/{id}` download by opaque 22-char base64url ID
- `HEAD /v1/blobs/{id}` metadata headers (`Content-Length`, `Content-Type`, `ETag`)
- FIFO eviction under capacity pressure
- tmpfs-aware capacity check using real filesystem free bytes (`statfs`)
- Single global upload gate for deterministic behavior under pressure
- Atomic visibility: blob becomes visible only after temp-write + `rename()` commit

## API

Base path: `/v1`

- `POST /v1/blobs`
  - Requires `Content-Length`
  - Rejects chunked uploads (`411 Length Required`)
  - Returns:

```json
{
  "id": "base64url_id",
  "size": 12345,
  "etag": "md5hex",
  "created_at": "RFC3339"
}
```

- `GET /v1/blobs/{id}`
  - `200` with blob bytes
  - `404` if missing/evicted/invalid ID

- `HEAD /v1/blobs/{id}`
  - `200` with headers
  - `404` if missing/evicted/invalid ID

- `GET /healthz`
  - `200 OK`

## Configuration

- `CAP_BYTES` (required)
- `DATA_DIR` (optional, default `/var/lib/blobbus`)
- `LISTEN_ADDR` (optional, default `:8080`)

## Local Run

```bash
CAP_BYTES=134217728 DATA_DIR=/tmp/blobbus LISTEN_ADDR=:8080 go run ./cmd/blobbus
```

## Tests

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
./scripts/integration.sh
```

## Docker

Build:

```bash
docker build -t blobbus:latest .
```

Run with tmpfs:

```bash
docker run \
  --tmpfs /var/lib/blobbus:rw,size=134217728 \
  -e CAP_BYTES=134217728 \
  -p 8080:8080 \
  blobbus:latest
```

## Kubernetes (memory-backed volume)

```yaml
volumes:
- name: blobbus-tmpfs
  emptyDir:
    medium: Memory
    sizeLimit: 128Mi

volumeMounts:
- name: blobbus-tmpfs
  mountPath: /var/lib/blobbus
```
