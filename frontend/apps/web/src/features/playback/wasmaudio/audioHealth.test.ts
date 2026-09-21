import { describe, it, expect } from "vitest";
import { AUDIO_HEAL_BACKOFF_MS, AUDIO_STUCK_MS, audioClockFrozen, healBackoffMs, stuckFor } from "./audioHealth";

// Prod, 2026-09-21: a phone browser lost the sound mid-film while the picture
// ran; a seek brought it back. These pin the two judgements that make such a
// session heal itself and leave a trace.

describe("audioClockFrozen", () => {
  it("re-bases on every tick while the clock moves", () => {
    const a = audioClockFrozen(null, 0, 10);
    expect(a.frozen).toBe(false);
    const b = audioClockFrozen(a.sample, 100, 10.1);
    expect(b.frozen).toBe(false);
    expect(b.sample).toEqual({ at: 100, ctxTime: 10.1 });
  });

  it("tolerates a short stall of the clock", () => {
    const a = audioClockFrozen(null, 0, 10);
    expect(audioClockFrozen(a.sample, AUDIO_STUCK_MS, 10).frozen).toBe(false);
  });

  it("reports a clock that has not moved for longer than the stuck window", () => {
    const a = audioClockFrozen(null, 0, 10);
    const b = audioClockFrozen(a.sample, AUDIO_STUCK_MS + 1, 10);
    expect(b.frozen).toBe(true);
    expect(b.sample).toEqual(a.sample); // the baseline is the last movement, not the last tick
  });

  it("clears the moment the clock moves again", () => {
    const a = audioClockFrozen(null, 0, 10);
    const b = audioClockFrozen(a.sample, AUDIO_STUCK_MS + 1, 10);
    expect(audioClockFrozen(b.sample, AUDIO_STUCK_MS + 200, 10.2).frozen).toBe(false);
  });
});

describe("stuckFor", () => {
  it("is zero while the condition does not hold", () => {
    expect(stuckFor(null, 5_000)).toBe(0);
  });
  it("measures from when the condition was first seen", () => {
    expect(stuckFor(1_000, 5_000)).toBe(4_000);
  });
});

describe("healBackoffMs", () => {
  it("grows with every attempt and caps at the last step", () => {
    expect(AUDIO_HEAL_BACKOFF_MS.map((_, i) => healBackoffMs(i))).toEqual(AUDIO_HEAL_BACKOFF_MS);
    expect(healBackoffMs(AUDIO_HEAL_BACKOFF_MS.length + 5)).toBe(AUDIO_HEAL_BACKOFF_MS[AUDIO_HEAL_BACKOFF_MS.length - 1]);
    expect(healBackoffMs(-1)).toBe(AUDIO_HEAL_BACKOFF_MS[0]);
  });
});
