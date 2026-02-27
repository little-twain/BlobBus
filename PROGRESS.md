# BlobBus Progress Alignment

## Goal
Deliver BlobBus according to `codex.md` milestones with incremental verifiable commits.

## Milestones
- [x] M1 Server skeleton
- [ ] M2 Core API (POST/GET/HEAD)
- [ ] M3 FIFO eviction (tmpfs-aware)
- [ ] M4 Determinism under pressure (single writer + ENOSPC retry)
- [ ] M5 Docker packaging
- [ ] M6 Single-command integration verification

## Verification Log
- 2026-02-27: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...` (pass)

## Notes
- Commit cadence: one commit per milestone.
- Existing untracked `codex.md` left untouched unless explicitly requested.
