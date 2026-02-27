# BlobBus

[![CI](https://github.com/little-twain/BlobBus/actions/workflows/ci.yml/badge.svg)](https://github.com/little-twain/BlobBus/actions/workflows/ci.yml)
[![Security](https://github.com/little-twain/BlobBus/actions/workflows/security.yml/badge.svg)](https://github.com/little-twain/BlobBus/actions/workflows/security.yml)
[![Docker Publish](https://github.com/little-twain/BlobBus/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/little-twain/BlobBus/actions/workflows/docker-publish.yml)

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

## Project Structure

```
BlobBus/
├── cmd/blobbus/main.go        # Application entry point
├── internal/blobbus/
│   ├── config.go              # Environment-based configuration
│   ├── errors.go              # Sentinel error definitions
│   ├── handler.go             # HTTP handlers (upload/download/head/health)
│   ├── id.go                  # Base64url ID generation and validation
│   └── store.go               # Blob storage engine with FIFO eviction
├── scripts/integration.sh     # End-to-end integration test script
├── Dockerfile                 # Multi-stage Docker build
└── go.mod                     # Go module definition
```

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

## CI/CD

GitHub Actions automates the full pipeline on every push and pull request:

| Workflow | Trigger | Description |
|---|---|---|
| **CI** | push / PR to `main` | Lint (`golangci-lint`), unit tests with race detection, build, integration tests |
| **Security** | push / PR to `main` + weekly schedule | `govulncheck` (Go vulnerability database), `gosec` (static security analysis), CodeQL |
| **Docker Publish** | push to `main` / version tags (`v*`) | Multi-arch Docker image (`linux/amd64`, `linux/arm64`) pushed to `ghcr.io` |

### Using the published Docker image

```bash
docker pull ghcr.io/little-twain/blobbus:main
docker run \
  --tmpfs /var/lib/blobbus:rw,size=134217728 \
  -e CAP_BYTES=134217728 \
  -p 8080:8080 \
  ghcr.io/little-twain/blobbus:main
```

To use a specific version tag:

```bash
docker pull ghcr.io/little-twain/blobbus:v1.0.0
```
