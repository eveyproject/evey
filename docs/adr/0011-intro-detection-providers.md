# ADR-0011: External intro providers before the heuristic pipeline

- Status: Accepted

## Context

Finding an intro locally means decoding audio and video to compute perceptual
hashes and fingerprints. It is the most expensive thing the scanner does, and
for popular content the answer is already known: community databases publish
intro and credits timestamps.

## Decision

A domain port, `IntroMetadataProvider`, is asked **first**, before the MKV
chapter pass and before any local detection. When it answers, the rest of the
pipeline is skipped entirely - that short-circuit is the point.

Behind the port sit two community sources chained first-non-empty, a
Postgres-backed cache, and a rate limiter that reads the remaining-quota headers
and pauses before hitting a refusal rather than after.

Cache lifetimes differ by outcome. A found answer is kept forever; a not-found
answer expires after thirty days, so content the community documents later
resurfaces instead of being permanently marked absent.

Contributing local detections back is opt-in, and only sends results the local
detector is highly confident about. A flood of weak submissions would degrade
the database everyone reads.

## Consequences

- On covered content the scanner does no audio or video decoding at all, which
  is the dominant saving on a weak machine.
- Adding a source means implementing the port and joining the chain; the
  pipeline does not change.
- The skip predicate must know which sources are authoritative, otherwise a
  re-scan reprocesses items already covered. Heuristic results stay eligible for
  replacement, community and chapter results do not.
- The server depends on a third party for a feature that must degrade quietly:
  when the source is unreachable the local detector runs as before.
