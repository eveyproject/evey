# ADR 0023 - Refresh-token rotation grace window (sibling sessions for race losers)

Status: Accepted
Supersedes: the earlier "strict single-use rotation" stance

## Context

Refresh tokens rotate on every use: `/auth/refresh` consumes the presented
token and issues a new pair. Browsers share ONE HttpOnly refresh cookie across
every tab. When several tabs refresh concurrently (all their access tokens
expire together after an idle stretch), only one rotation can win; the losers
present a token that was consumed milliseconds earlier.

The pipeline had already walked back one step of strictness: replay detection
originally purged the user's *entire* session set, and every purge observed in
staging was a false positive (a race loser or an interrupted rotation), so the
purge was dropped. But the loser itself was still rejected with 401 - which
kills that tab outright: every API call fails (profile, notifications, images)
until a manual reload. Observed live on the media-requests page; an earlier
decision had accepted this as a dev-only annoyance, and it proved to hit real
usage.

## Decision

A rotated refresh token remains exchangeable for **30 seconds** after its
rotation (`refreshRotationGrace`). Within the grace, the presenter is treated
as a multi-tab race loser and issued a **sibling session**: its own fresh
access + refresh pair. The winner's tokens are untouched; the two sessions
live independently thereafter.

Mechanics:

- Rotation stamps `refresh_tokens.used_at` (migration 000135) and the claimed
  row is **retained** instead of deleted - it must stay findable for the grace
  check. `DeleteExpired` sweeps used rows once the grace has long passed.
- `validateRefreshToken` grants the grace only when `used_at` is within the
  window. A used row with **no** timestamp (legacy row claimed before the
  column existed) fails closed. Past the window, the token is a replay and is
  rejected - without purging other sessions, per the earlier decision.
- A racer that loses the atomic claim *mid-flight* (validated the row before
  the winner committed) falls through to the same sibling issue: it lost by
  milliseconds, squarely within any grace.

## Consequences

- N tabs refreshing concurrently all succeed with N distinct sessions
  (`TestRefreshToken_ConcurrentMultiTabRefresh_AllGetDistinctSessions` pins
  this; the old TOCTOU test asserted exactly-one-winner and was re-anchored).
- **Accepted trade-off**: a refresh token stolen AND replayed within 30 s of a
  legitimate rotation yields a session. Outside that window nothing changes.
  This is the industry-standard reuse-leeway compromise, and it is strictly
  bounded - unlike the tab-killing 401, which was unbounded user pain.
- Session rows grow by one per race loser; bounded by the sweep and by token
  expiry.
