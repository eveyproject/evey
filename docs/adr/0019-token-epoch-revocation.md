# ADR-0019: Token-epoch revocation for stateless JWTs (revoke without a blocklist)

- Status: Accepted

## Context

Evey's access tokens are stateless JWTs (ADR-0003 covers the plugin variant): the server validates a signature and trusts the claims, no DB lookup per request. This is fast and horizontally trivial, but it has the classic stateless-JWT weakness: **a token cannot be revoked before it expires.** When an admin disables a user, resets a password, or changes a role, that user's already-minted access tokens stay valid until natural expiry.

The two textbook fixes both have costs:

1. **A token blocklist** (store every revoked token id, check it on each request). Re-introduces a per-request DB/cache read of an ever-growing set, and the set must be retained until each token's expiry. Heavy for the common case (no revocations).
2. **Very short token lifetimes + refresh.** Shrinks the window but never closes it, and hammers the refresh path. The refresh token itself still needs revocation.

We want revocation that is **fail-closed**, costs ~nothing in the common case, and does not turn every request into a DB read.

## Decision

Each user carries a monotonically increasing **token epoch** (an integer column, migration 000114). Every minted access token embeds the current epoch as an `epoch` claim. Validation accepts the token only if its `epoch` matches the user's current epoch - looked up through a short-TTL cache. A revocation is a single **epoch bump**, which invalidates every token minted before it in one write.

**One decorator, one wire instance.** `EpochGuardedTokens` decorates the `TokenGenerator` port. A single instance is wired (`cmd/evey/wire.go`) and used by every mint site - gRPC, HTTP, all `register*` flows - so there is exactly one place that stamps and checks the epoch. Mint stamps the current epoch; validate is **fail-closed** (a lookup error denies); Revoke bumps the epoch and drops the cache entry.

**A narrow `TokenEpochRepository` port.** Epoch read/bump is a dedicated narrow port, deliberately kept out of `UserRepository.Update`/`userColumns` so a routine user update never accidentally moves the epoch. The bump is atomic.

**30-second cache TTL: the latency/cost knob.** The epoch lookup is cached for 30 s. This bounds the revocation latency (a disabled user keeps access for at most ~30 s after the TTL elapses) while keeping validation a cache hit, not a DB read, in the overwhelmingly common no-revocation case. 30 s was chosen as the security-vs-cost balance: short enough that "disable user" feels immediate, long enough that the cache absorbs request bursts.

**What bumps the epoch (security events only - NOT logout).** `DisableUser`, `ChangePassword`, `ResetPassword`, and role changes (`UpdateUser` when the role moves) bump the epoch. `DeleteUser` is covered by the lookup failing closed. Ordinary logout does **not** bump (it just drops the refresh token) - the epoch is for *security* revocations, not session hygiene, so a logout on one device doesn't kill every other session.

**Legacy tokens.** Tokens minted before this mechanism carry no epoch (epoch 0) and remain valid until the first bump for that user. There is no flag day.

## Consequences

- **Positive:** real revocation on a stateless JWT model with O(1) revocation (one bump) and a cache-hit validation path; no growing blocklist; fail-closed by construction.
- **Cost:** the JWT is no longer *purely* stateless - validation now consults the (cached) epoch, so a node with a cold cache pays one DB read per user per 30 s window. Up to ~30 s revocation latency by design. Every new token-issuance path MUST go through the single `EpochGuardedTokens` instance, or it will mint tokens that bypass revocation. Credential revocation (`RevokeUserTokens` = epoch bump + refresh-token delete) is triggered by the security mutations - `DisableUser`, `ChangePassword`, `ResetPassword`, role change, `DeleteUser`. It is deliberately NOT triggered by `KillSession`: stopping a playback session must not sign the user out of the app (banning a user is the separate `DisableUser` action).
- Plugin tokens keep a separate generator (`PluginTokenGen`, raw) - they are scoped/short-lived per ADR-0003 and out of this mechanism's scope.
