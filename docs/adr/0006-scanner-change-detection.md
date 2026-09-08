# ADR-0006: Scanner change-detection contract

- Status: Accepted
- Date: 2026-04-20

## Context

The periodic library scanner decides, at every tick (every 2 minutes), whether each file on disk is already represented by a row in the database. A false positive ("changed" when unchanged) is catastrophic: every file re-enters the full upsert + ffprobe + meili-index + event-bus pipeline. On a modest library this pegged one CPU core at 100% for an entire weekend (see the five scan-related commits on 2026-04-20).

Three independent sources of false-positive change-detection surfaced in the same week:

1. `time.Time` rounding drift. Postgres `TIMESTAMPTZ` stores microseconds with half-up rounding on sub-µs input; Go's `Truncate(time.Microsecond)` rounds down. An ext4 mtime of `.8474756` (100ns precision) persists as `.847476` and re-reads as `.847475` after `Truncate(µs)` - the two are compared, fail equality, and every ext4 file is flagged changed every scan.
2. `File.ModTime` not persisted at `Create`. The field was missing from the struct literal in `createMediaFile`; 464 rows held `0001-01-01` on disk, and every scan saw a zero-time vs real-time mismatch.
3. Fingerprint written only after a long probe. `updateMediaFile` ran `ProbeAndStoreStreams` + `extractAndStoreChapters` on the full 46 GB MKV before calling the row `Update`. The 30s worker context expired during the probe, the `Update` aborted with `begin tx: context deadline exceeded`, and the new fingerprint was never persisted. Next scan saw the old fingerprint and re-entered the same loop.

These are not bugs in domain logic - they are violations of an implicit contract about what "changed" means and in what order the scanner must persist its decisions.

## Decision

Formalise the contract. Any file-backed scanner (TV/movie, photo, music, audiobook, ...) must satisfy the four rules below.

**Rule 1 - `sameMtime` compares at µs with ±1µs tolerance.**

```go
func sameMtime(a, b time.Time) bool {
    at := a.Truncate(time.Microsecond)
    bt := b.Truncate(time.Microsecond)
    if at.Equal(bt) {
        return true
    }
    return at.Sub(bt) == time.Microsecond || bt.Sub(at) == time.Microsecond
}
```

Two one-directional `Sub == time.Microsecond` checks avoid `MinInt64` overflow when one operand is a zero `time.Time` (e.g. a freshly-created row whose `File.ModTime` hasn't been populated yet). Never `Round(µs).Equal()`: Postgres's rounding rule is not guaranteed to match Go's across driver versions.

**Rule 2 - every scanner-mutated field must be set at `Create`.** Whatever the scanner compares on the next pass must be written on the first. At minimum: `Path`, `Size`, `Fingerprint`, `ModTime`. A missing field stored as a zero value forces a re-upsert on every subsequent scan.

**Rule 3 - dedup state persists before any side-effect.**

```
1. Mutate in-memory struct (fingerprint, mtime, size, path).
2. s.mediaItems.Update(ctx, item)   ← MUST succeed for next-scan skip to work
3. ProbeAndStoreStreams / extractChapters / IndexMediaItem / publish events
```

If step 3 times out, next scan loses the duration refresh or the meili reindex - recoverable side-effects. If step 2 times out because step 3 was moved ahead, next scan re-processes the same file forever - the non-recoverable failure mode. The critical path is the dedup row; everything else is best-effort.

**Rule 4 - `workerTimeout` bounds the slowest legitimate path, not the average.** `processNewItems` spawns one goroutine per scanWork with its own `WithTimeout(ctx, workerTimeout)`. The timeout must accommodate `ffprobe + extractChapters + Update` on the largest UHD file in the library, not the median case. We currently use **5 minutes**; 30 seconds was enough for 1080p but lost the race on 2160p HEVC 40GB+ files.

## Consequences

- **Test coverage.** `scan_service_test.go` has a pair of scans on the same file fixture asserting `added=1; second_scan_updated=0`. The old assertion `updated=1` (that documented the buggy re-upsert path) has been inverted to enforce the contract.
- **scan_title is complementary.** `scan_title` gives us a *stable* dedup key even when enrichment rewrites `Title`; see `scan_service_tvshow.go` series/season dedup. This ADR is about per-scan change-detection; `scan_title` is about cross-enrichment identity.
- **New scanner types inherit the rules.** Photo and music scanners both carry an inline `sameMtime`; duplicating across three packages is intentional - a single shared helper would sit in an odd layer (neither `domain/library` nor `pkg/`) and block the `photo`/`music` domains from importing independently.
- **The audit tool learned.** An explicit diagnostic (`cmd/photo-diag`) was used to validate the fix on the live database; keep that pattern available when any future scan change-detection regression surfaces.

## References

- `internal/app/library/scan_service.go` - `sameMtime`, `updateMediaFile`, `processNewItems`
- `internal/app/photo/command_service.go` - mirrors the contract
- `internal/app/music/command_service.go` - mirrors the contract
- `internal/app/library/scan_service_test.go` - `TestScanLibrary_ExistingFile_NoDuplicate`
- Commits 2026-04-20: `fix(scan): ±1µs tolerance`, `fix(scan): persist File.ModTime`, `fix(scan): persist fingerprint BEFORE ffprobe`

## Amendment (2026-05-23)

The change-detection **rules** above still hold, but two Consequences claims were undone:

- The "inline `sameMtime` duplicated across three packages is intentional, a shared helper would sit in an odd layer" rationale was **reversed**. The helper was extracted to a shared package, `internal/pkg/scanindex.SameMtime`, now used by the library, photo, and music scanners. The odd-layer concern was resolved by putting it in `pkg/`, not `domain/`.
- `cmd/photo-diag` was never shipped; disregard that reference.
