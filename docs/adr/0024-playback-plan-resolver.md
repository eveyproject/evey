# ADR 0024 - App-layer PlaybackPlan resolver (one resolution per request)

Status: Accepted

## Context

The HLS read path answers, per request, "how do we serve this stream": the
per-track copy/transcode decision with its request-time pins, the shaped ABR
ladder, the hardware route, the copy-rung routing, the producer pick, and the
`DecisionFingerprint` that keys the segment cache. Historically this brain
lived spread across ~8 `transcode_vod_*.go` files in the **http driving
adapter**, and every consumer re-derived its inputs independently: a single
segment request recomputed the decision up to three times (bound check +
fingerprint + producer pick), each reloading the profile preferences from
Postgres - measured at 5 prefs fetches per segment, 6 per master.

Correctness rested on the discipline that every derivation produce
byte-identical inputs (the *cache-hash invariant*): any drift between the
manifest's view and a segment's view desyncs URLs from bytes (ADR 0022's
incident class).

## Decision

Resolve the whole plan **once per request** in the app layer:

- `PlaybackRequest` (transport-free) carries everything the decision needs:
  source facts, client capabilities, profile prefs, and the request-time pins.
  The http adapter's only decision-adjacent job is marshalling a
  `*http.Request` into it (`marshalPlaybackRequest` - the single place where a
  query parameter becomes a plan input).
- `PlanResolver.Resolve(ctx, req) → PlaybackPlan` (
  `internal/app/transcoding/playback_plan_resolver.go`) computes decision →
  ladder → fingerprint in that fixed order (the shaping creates the admission
  pin; the fingerprint binds to it) and returns an **immutable** plan: decision,
  ladder class, shaped ladder, effective codec/HW route, packager kind,
  fingerprint + hash.
- Every consumer - manifest input, shed probe, stale-URL seal, producer pick,
  segment cache key - reads that single resolution. Within a request the
  cache-hash invariant holds **by construction** (one derivation, not N that
  must agree). Across requests it still rests on URL baking + the frozen
  ladder (ADR 0022).
- All resolver dependencies are nil-safe: a zero-value resolver reproduces the
  historical safe fallbacks (720p software ceiling, no admission, no copy
  rung), so hand-built test handlers keep their contract.

The profile preferences behind the resolution are cached for 10 s per profile
(same staleness contract as the media lookup cache), taking the segment hot
path from 5 Postgres round-trips to ~0 within a playback burst.

## Consequences

- The brain is testable without `net/http`; wiring a new codec, packager or
  container is one change in the resolver instead of a sweep across the
  adapter.
- The migration was proven byte-stable: the decision fingerprint of a live
  stream is identical before/after the move (no cache invalidation), and the
  pre-existing ~10k-line adapter test suite runs unchanged against delegating
  wrappers.
- The zero-process mkvgo paths (on-demand `od_`/`cc_` resources) are
  dispatched before the resolver runs; the resolver's packager axis only ever
  bills the two ffmpeg packagers.
- Reading order for the whole path: [docs/playback-serving.md](../playback-serving.md).
