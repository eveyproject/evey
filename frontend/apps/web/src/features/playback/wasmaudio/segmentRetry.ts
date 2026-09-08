// Backoff/attempt-cap policy for WasmAudioEngine's decode pump loop retrying a
// FAILED segment. Split out as pure, timestamp-injected state so the failure
// path (pump()) stays a thin caller instead of embedding untested timing math.
//
// Prod, 2026-09-08: the 5-minute sliding stream token expired during a pause
// longer than that; on resume hls.js had enough buffer to ask for nothing,
// but the wasm audio leg's shorter look-ahead fetched first and got 401. With
// no backoff and no way to heal, the pump loop re-fetched the SAME index at
// RTT speed forever - 11,884 rejected requests in a day, bursts of
// 3,088/minute. This tracker is the missing seam: it decides whether an index
// may be retried yet, and whether a 401 should ask for a fresh token before
// backing off.

/** Base backoff before a failed segment's index may be retried. */
export const RETRY_BASE_MS = 500;
/** Backoff never grows past this - an unhealthy segment still gets retried,
 * just no faster than once every 10s. */
export const RETRY_MAX_MS = 10_000;
/** After this many failures the index is given up on for the rest of the
 * session: whatever is wrong with it isn't timing, and grinding on it forever
 * denies the rest of the look-ahead window its decode budget. */
export const RETRY_MAX_ATTEMPTS = 6;

/** Exponential backoff for the (attempts+1)-th attempt (attempts = failures
 * already recorded for this index), capped at RETRY_MAX_MS. */
export function nextRetryDelayMs(attempts: number): number {
  const n = Math.max(0, attempts);
  return Math.min(RETRY_BASE_MS * 2 ** n, RETRY_MAX_MS);
}

interface RetryEntry {
  attempts: number;
  nextAttemptAt: number;
  authRefreshTried: boolean;
}

/**
 * SegmentRetryTracker holds per-index failure state for the pump loop. All
 * timing is passed in via `now` rather than read internally, so it is a pure,
 * directly-testable state machine - no fetch, no timers, no engine.
 */
export class SegmentRetryTracker {
  private readonly entries = new Map<number, RetryEntry>();

  /** May idx be attempted at time `now`? False while it is still backing off,
   * or once it has exhausted its attempt budget. */
  canAttempt(idx: number, now: number): boolean {
    const e = this.entries.get(idx);
    if (!e) return true;
    if (e.attempts >= RETRY_MAX_ATTEMPTS) return false;
    return now >= e.nextAttemptAt;
  }

  /** True once idx has exhausted RETRY_MAX_ATTEMPTS - distinct from a mere
   * backoff wait, for callers that want to log/report giving up on it. */
  exhausted(idx: number): boolean {
    return (this.entries.get(idx)?.attempts ?? 0) >= RETRY_MAX_ATTEMPTS;
  }

  /**
   * Records a failed attempt at time `now` and schedules the next one via
   * exponential backoff. Returns true exactly once per failure streak on an
   * auth error (401/403) - the caller should ask for a fresh token, then
   * still respect the backoff: a refresh does not bypass it, in case the
   * failure was not actually auth-related.
   */
  recordFailure(idx: number, now: number, isAuthError: boolean): boolean {
    const e = this.entries.get(idx) ?? { attempts: 0, nextAttemptAt: now, authRefreshTried: false };
    e.nextAttemptAt = now + nextRetryDelayMs(e.attempts);
    e.attempts += 1;
    const askForRefresh = isAuthError && !e.authRefreshTried;
    if (askForRefresh) e.authRefreshTried = true;
    this.entries.set(idx, e);
    return askForRefresh;
  }

  /** A successful fetch/decode clears whatever failure history idx had. */
  clear(idx: number): void {
    this.entries.delete(idx);
  }

  /** Drops every entry - used when the timeline is replaced (hand-over,
   * revert) and old indices no longer name the same segments. */
  clearAll(): void {
    this.entries.clear();
  }
}
