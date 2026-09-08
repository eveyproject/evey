import { describe, it, expect } from "vitest";
import { nextRetryDelayMs, RETRY_BASE_MS, RETRY_MAX_MS, RETRY_MAX_ATTEMPTS, SegmentRetryTracker } from "./segmentRetry";

// Prod, 2026-09-08: an expired stream token 401'd every segment fetch and, with
// no backoff, the pump loop re-fetched the SAME index at RTT speed - 11,884
// rejected requests in a day. These pin the fix: bounded, backed-off retries,
// and a 401 asking for a fresh token exactly once per failure streak.

describe("nextRetryDelayMs", () => {
  it("grows exponentially from the base", () => {
    expect(nextRetryDelayMs(0)).toBe(RETRY_BASE_MS);
    expect(nextRetryDelayMs(1)).toBe(RETRY_BASE_MS * 2);
    expect(nextRetryDelayMs(2)).toBe(RETRY_BASE_MS * 4);
  });

  it("caps at RETRY_MAX_MS", () => {
    expect(nextRetryDelayMs(10)).toBe(RETRY_MAX_MS);
    expect(nextRetryDelayMs(100)).toBe(RETRY_MAX_MS);
  });

  it("clamps a negative attempt count to the base delay", () => {
    expect(nextRetryDelayMs(-1)).toBe(RETRY_BASE_MS);
  });
});

describe("SegmentRetryTracker", () => {
  it("allows an index that has never failed", () => {
    const t = new SegmentRetryTracker();
    expect(t.canAttempt(3, 0)).toBe(true);
  });

  it("backs off after a failure and allows retry once the delay elapses", () => {
    const t = new SegmentRetryTracker();
    t.recordFailure(3, 1_000, false);
    expect(t.canAttempt(3, 1_000)).toBe(false);
    expect(t.canAttempt(3, 1_000 + RETRY_BASE_MS - 1)).toBe(false);
    expect(t.canAttempt(3, 1_000 + RETRY_BASE_MS)).toBe(true);
  });

  it("doubles the backoff on each consecutive failure", () => {
    const t = new SegmentRetryTracker();
    let now = 0;
    t.recordFailure(1, now, false); // -> +500
    now += RETRY_BASE_MS;
    expect(t.canAttempt(1, now)).toBe(true);
    t.recordFailure(1, now, false); // -> +1000
    expect(t.canAttempt(1, now + RETRY_BASE_MS * 2 - 1)).toBe(false);
    expect(t.canAttempt(1, now + RETRY_BASE_MS * 2)).toBe(true);
  });

  it("stops allowing retries once attempts are exhausted", () => {
    const t = new SegmentRetryTracker();
    let now = 0;
    for (let i = 0; i < RETRY_MAX_ATTEMPTS; i++) {
      t.recordFailure(5, now, false);
      now += RETRY_MAX_MS; // fast-forward well past any backoff
    }
    expect(t.exhausted(5)).toBe(true);
    expect(t.canAttempt(5, now + 1_000_000)).toBe(false);
  });

  it("asks for a refresh exactly once per failure streak on a 401", () => {
    const t = new SegmentRetryTracker();
    let now = 0;
    expect(t.recordFailure(2, now, true)).toBe(true);
    now += RETRY_MAX_MS;
    // Same index failing again with another auth error must not ask again -
    // one refresh call per streak, not one per failed request.
    expect(t.recordFailure(2, now, true)).toBe(false);
  });

  it("does not ask for a refresh on a non-auth failure", () => {
    const t = new SegmentRetryTracker();
    expect(t.recordFailure(4, 0, false)).toBe(false);
  });

  it("a successful fetch clears the state for that index", () => {
    const t = new SegmentRetryTracker();
    t.recordFailure(7, 0, true);
    expect(t.canAttempt(7, 0)).toBe(false);
    t.clear(7);
    expect(t.canAttempt(7, 0)).toBe(true);
    // A fresh failure after clearing is allowed to ask for a refresh again -
    // clearing resets the whole streak, not just the backoff.
    expect(t.recordFailure(7, 0, true)).toBe(true);
  });

  it("clearAll drops every tracked index", () => {
    const t = new SegmentRetryTracker();
    t.recordFailure(1, 0, false);
    t.recordFailure(2, 0, false);
    t.clearAll();
    expect(t.canAttempt(1, 0)).toBe(true);
    expect(t.canAttempt(2, 0)).toBe(true);
  });
});
