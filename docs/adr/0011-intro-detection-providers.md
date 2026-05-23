# ADR-0011: External intro detection providers ahead of the heuristic pipeline

- Status: Accepted
- Date: 2026-05-04

## Context

Local intro detection (pHash + Chromaprint) is CPU-heavy: each season run extracts audio fingerprints across every sibling via ffmpeg, burning ~1 core per in-flight detection for several minutes. On large catalogues this is the dominant load post-scan. Two community databases publish curated intro / recap / credits timestamps for popular content keyed by external IDs:

- **TheIntroDB (TIDB)** — keyed by `tmdb_id`, supports movies and TV. 30 req / 10 s burst, 100/day anonymous, 500/day with Bearer auth. Returns intro / recap / credits / preview segments with millisecond timestamps. Documents `X-RateLimit-Remaining` / `X-RateLimit-Reset` headers.
- **IntroDB** — keyed by `imdb_id`, TV only. Three segment types (intro / recap / outro). Vague rate limits, no headers.

TMDB itself does **not** expose intro/outro metadata in any public API — that's a common assumption worth dispelling.

Submitting a `cmd run` of pHash for an item TIDB already covers wastes minutes of CPU for no marginal accuracy gain (TIDB's submissions are community-validated to a comparable confidence). The architecture should query the external DBs first and short-circuit when they return data.

## Decision

Introduce `domainlibrary.IntroMetadataProvider` as a port on the domain side:

```go
type IntroMetadataProvider interface {
    GetSegments(ctx context.Context, item *MediaItem) ([]MediaSegment, error)
}
```

Wire two concrete adapters and a chain decorator under `internal/adapters/driven/introproviders/`:

1. `TIDBClient` — primary, queries TheIntroDB by `tmdb_id`.
2. `IntroDBClient` — fallback for `imdb_id`-only items.
3. `ChainedProvider` — first non-empty result wins; per-source 10-min cooldown after a 429.
4. `Cached` — postgres-backed (`intro_metadata_cache` table, migration 000093). Positive results are cached forever, negative results (empty arrays / 404) for 30 days so newly-added community data eventually resurfaces.
5. `headerRateLimiter` — observes `X-RateLimit-Remaining` / `X-RateLimit-Reset` from each TIDB response and pauses proactively when remaining drops below 3, avoiding 429s on bulk scans.
6. `TIDBContributor` (opt-in) — implements `domainlibrary.SegmentContributor` and submits high-confidence local pHash detections (≥0.95) back to TheIntroDB for the community's benefit.

The intro detection pipeline calls the chain **before** the MKV chapter pass and the heuristic passes. When the chain returns non-empty segments, the rest of the pipeline is short-circuited and the local pHash + Chromaprint extraction is skipped — the dominant CPU saver on covered content.

`hasGoodIntro` (the backfill skip predicate) recognises segments from authoritative sources — `mkv_chapter`, `tidb`, `introdb`, `phash` — so a re-run never reprocesses items already covered. The audio-fallback (`audio`) and blackdetect (`blackdetect`) sources stay re-eligible since they're last-resort heuristics that should be replaced when better data lands.

Admin surface:

- `intro_providers.enabled` toggle in server-config — restart required (providers are constructed once at boot).
- `api.tidb_key` field — DB-first with env fallback (`EVEY_INTRO_TIDB_API_KEY`), same `resolveAPIKey` pattern as TMDB / OMDB / OpenSubtitles keys.
- `intro_providers.tidb_submit_enabled` opt-in toggle for community contribution.
- `DELETE /api/v1/admin/intro-cache/{tmdb_id}/{season}/{episode}` — force a refetch on a stale row.
- `POST /api/v1/admin/intro-cache/backfill` — non-blocking targeted backfill scoped to one library (or all). Mutex-protected against double-runs.

CPU budget: the same `cpuutil.TrickplayWorkers(adaptive)` cap that gates trickplay generation also gates parallel intro detections, and every intro-related ffmpeg invocation runs with `-threads 1` (override via `EVEY_INTRO_FFMPEG_THREADS`). The video player is unaffected.

## Consequences

- Provider chain is the **first** decision point in the pipeline — adapters at the network boundary can short-circuit work that would otherwise hit the disk and the CPU.
- Cache invariants matter: the `Cached` decorator owns the negative-TTL contract; the postgres schema stores `fetched_at` so the decorator can age out stale negative rows.
- New providers (AniDB, simkl, plugin DB) plug in by implementing `Client` and joining the chain — no pipeline change.
- Submission is **opt-in** by design. We don't want a flood of low-confidence submissions degrading the community DB; only `phash` segments with `Confidence ≥ 0.95` are sent.
- Tests stay fast: every adapter takes a `WithBaseURL` option pointing at a `httptest` server; the rate-limiter takes injectable `nowFunc` / `sleepFunc` so pacing tests don't actually sleep.

## Update 2026-05-23 — TIDB API v3

TheIntroDB shipped a v3 API (npm `theintrodb@3.0.0`, 2026-05-19; the official Jellyfin plugin pins `/v3`). v2 still serves identical-shaped responses, so the migration is non-breaking. `TIDBClient` now targets `https://api.theintrodb.org/v3`. Changes adopted on the **read** path (`Fetch`):

- **imdb_id fallback** — v3 accepts `imdb_id` directly (movies and series), so `TIDBClient` queries by `imdb_id` when no `tmdb_id` is resolvable instead of bailing straight to `IntroDBClient`. The IntroDB (`introdb.app`) adapter stays as a secondary fallback but is now partly redundant.
- **`duration_ms`** — forwarded from the item runtime (`Lookup.DurationMs`, sourced from `MediaItem.File.Duration`). Lets the server match the right release cut and resolve open-ended segments.
- **Open-ended credits/preview kept** — `end_ms == null` (= "to end of media") is now closed against `Lookup.DurationMs` instead of being dropped. intro/recap still require an explicit end. Previously every null-end credit was silently discarded.
- **Confidence** — v3 may attach per-segment `confidence`/`submission_count`; `Fetch` uses `confidence` when present (clamped 0..1), else the 0.97 floor.
- **Rate-limit tiers** — v3 exposes three buckets: `X-RateLimit-*` (30/10s burst, paced proactively), `X-UsageLimit-*` (100/500 per day), `X-UsageLimit-SpecificMedia-*` (~1000/media). `parseRetryAfter` now returns the longest of `Retry-After` / `X-UsageLimit-Reset` / `X-RateLimit-Reset` so a daily-cap 429 isn't retried inside the burst window. The daily/per-media tiers are not paced proactively (reset windows can be hours); a cap 429 terminates the run and resumes next cycle.
- **User-Agent** — requests now send a named UA (`tidbUserAgent`).

**Submission (`/submit`) deliberately left on the v2 path and unchanged.** The v3 submit schema differs (`segment`/`start_ms`/`end_ms`/`type`, segment vocab `intro|recap|credits|preview` with no `outro`; our payload sends `segment_type`/`start_sec`/`end_sec`/`outro` and omits `type`). Migrating the submit path is deferred until we revisit community contribution.

## References

- `internal/domain/library/ports.go` — `IntroMetadataProvider`, `SegmentContributor`
- `internal/adapters/driven/introproviders/` — clients, chain, cache, rate-limiter, contributor
- `internal/adapters/driven/postgres/intro_metadata_cache_repo.go` + migration `000093_intro_metadata_cache.up.sql`
- `internal/app/library/intro_detection_service.go` — pipeline integration + `BackfillLibrary` admin entry point
- `proto/evey/v1/admin_intro.proto` — invalidation + backfill endpoints
- Commits 2026-05-04 / 2026-05-05 (handoff plans A–E in session memory).
