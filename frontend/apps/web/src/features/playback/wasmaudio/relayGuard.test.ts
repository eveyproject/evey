import { describe, it, expect } from "vitest";
import { shouldHandover, isStarved, DEFAULT_STARVATION_MS } from "./relayGuard";

// Prod, 2026-07-30, a weak quad-A53 operator box + E-AC-3 7.1: the relay handed over the moment
// the wasm worker said "ready", the decoder could not keep up, the sound died
// and the box killed the receiver. Twice. These pin both guards.

describe("shouldHandover", () => {
  it("accepts a decoder comfortably faster than real time", () => {
    // 5s segment decoded in 900ms = 0.18x real time.
    expect(shouldHandover({ decodeMs: 900, segmentDurSec: 5, channels: 6 })).toEqual({ ok: true });
  });

  it("refuses a decoder that cannot sustain real time", () => {
    const v = shouldHandover({ decodeMs: 4500, segmentDurSec: 5, channels: 6 });
    expect(v.ok).toBe(false);
    expect(v.reason).toContain("real time");
  });

  it("refuses the 7.1 case that broke prod, even when the timing looks fine", () => {
    const v = shouldHandover({ decodeMs: 500, segmentDurSec: 5, channels: 8 });
    expect(v.ok).toBe(false);
    expect(v.reason).toContain("8 channels");
  });

  it("refuses anything it cannot judge", () => {
    // Refusing costs audio quality; accepting a bad hand-over costs the film.
    expect(shouldHandover({ decodeMs: 100, segmentDurSec: 0, channels: 6 }).ok).toBe(false);
    expect(shouldHandover({ decodeMs: 100, segmentDurSec: 5, channels: 0 }).ok).toBe(false);
    expect(shouldHandover({ decodeMs: Number.NaN, segmentDurSec: 5, channels: 6 }).ok).toBe(false);
  });

  it("honours caller-supplied limits", () => {
    expect(shouldHandover({ decodeMs: 4500, segmentDurSec: 5, channels: 6, maxRealtimeRatio: 0.95 }).ok).toBe(true);
    expect(shouldHandover({ decodeMs: 500, segmentDurSec: 5, channels: 8, maxChannels: 8 }).ok).toBe(true);
  });

  it("sits exactly on the limit without flapping", () => {
    // 0.7x exactly is accepted; a hair over is not.
    expect(shouldHandover({ decodeMs: 3500, segmentDurSec: 5, channels: 6 }).ok).toBe(true);
    expect(shouldHandover({ decodeMs: 3600, segmentDurSec: 5, channels: 6 }).ok).toBe(false);
  });
});

describe("isStarved", () => {
  const playing = { switched: true, playing: true };

  it("fires when the wasm leg has delivered nothing while the picture runs", () => {
    expect(isStarved({ ...playing, sinceLastDecodeMs: DEFAULT_STARVATION_MS + 1 })).toBe(true);
  });

  it("tolerates one slow segment", () => {
    expect(isStarved({ ...playing, sinceLastDecodeMs: 6_000 })).toBe(false);
  });

  it("never fires on a paused player", () => {
    // Paused decodes nothing, legitimately - falling back there would be a bug
    // that hits every user who pauses for a coffee.
    expect(isStarved({ switched: true, playing: false, sinceLastDecodeMs: 60_000 })).toBe(false);
  });

  it("never fires before the hand-over", () => {
    // The AAC leg is playing; the wasm leg is allowed to take its time booting.
    expect(isStarved({ switched: false, playing: true, sinceLastDecodeMs: 60_000 })).toBe(false);
  });

  it("honours a caller-supplied threshold", () => {
    expect(isStarved({ ...playing, sinceLastDecodeMs: 3_000, thresholdMs: 2_000 })).toBe(true);
  });

  // Prod, 2026-09-20: a viewer seeked back into audio the engine had already
  // decoded, the pump had nothing left to fetch for 32 s, and a leg playing 4
  // scheduled segments at 0 ms drift was declared starved and latched to AAC
  // for the rest of the film.
  it("never fires on a leg that owes nothing", () => {
    expect(isStarved({ ...playing, sinceLastDecodeMs: 60_000, pending: false })).toBe(false);
  });

  it("still fires when the leg owes a segment it has not produced", () => {
    expect(isStarved({ ...playing, sinceLastDecodeMs: DEFAULT_STARVATION_MS + 1, pending: true })).toBe(true);
  });
});
