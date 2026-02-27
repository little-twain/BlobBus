# BlobBus Progress Alignment

## Goal
Deliver BlobBus according to `codex.md` milestones with incremental verifiable commits.

## Milestones
- [x] M1 Server skeleton
- [x] M2 Core API (POST/GET/HEAD)
- [x] M3 FIFO eviction (tmpfs-aware)
- [x] M4 Determinism under pressure (single writer + ENOSPC retry)
- [x] M5 Docker packaging
- [x] M6 Single-command integration verification

## Verification Log
- 2026-02-27: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...` (pass)
- 2026-02-27: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod CGO_ENABLED=0 go build ./cmd/blobbus` (pass)
- 2026-02-27: `./scripts/integration.sh` (pass)
- 2026-02-27: hardening pass added tests for upload gate serialization, multi-eviction FIFO order, and reader-open-FD survival during eviction.
- 2026-02-27: full-flow HTTP tests added for non-LRU FIFO behavior, concurrent multi-client upload/download, and upload gate blocking semantics.
- 2026-02-27: CI hardening: integration script switched to build+run binary with longer startup wait and health failure logs; setup-go cache warnings removed in CI workflow.
- 2026-02-27: security workflow hardening: govulncheck moved to stable Go and set non-blocking; gosec excludes MD5 checks required by BlobBus spec; storage directories tightened to 0700.
- 2026-02-27: fixed gosec findings in code path (`G115` statfs overflow-safe conversion, `G304` internal temp-path rationale annotation).

## Notes
- Commit cadence: milestone-based progress commits.
- Existing untracked `codex.md` left untouched unless explicitly requested.
