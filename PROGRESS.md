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
- 2026-02-27: `./scripts/integration.sh` (pass)

## Notes
- Commit cadence: milestone-based progress commits.
- Existing untracked `codex.md` left untouched unless explicitly requested.
