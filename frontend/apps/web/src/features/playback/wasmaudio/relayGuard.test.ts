import { describe, it, expect } from "vitest";
import { shouldHandover, isStarved, owedSince, DEFAULT_STARVATION_MS } from "./relayGuard";

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

  it("fires when the wasm leg has owed a segment for too long while the picture runs", () => {
    expect(isStarved({ ...playing, owedForMs: DEFAULT_STARVATION_MS + 1 })).toBe(true);
  });

  it("tolerates one slow segment", () => {
    expect(isStarved({ ...playing, owedForMs: 6_000 })).toBe(false);
  });

  it("never fires on a paused player", () => {
    // Paused decodes nothing, legitimately - falling back there would be a bug
    // that hits every user who pauses for a coffee.
    expect(isStarved({ switched: true, playing: false, owedForMs: 60_000 })).toBe(false);
  });

  it("never fires before the hand-over", () => {
    // The AAC leg is playing; the wasm leg is allowed to take its time booting.
    expect(isStarved({ switched: false, playing: true, owedForMs: 60_000 })).toBe(false);
  });

  it("honours a caller-supplied threshold", () => {
    expect(isStarved({ ...playing, owedForMs: 3_000, thresholdMs: 2_000 })).toBe(true);
  });

  it("never fires on a leg that owes nothing", () => {
    expect(isStarved({ ...playing, owedForMs: 0 })).toBe(false);
  });
});

describe("owedSince", () => {
  it("charges nothing while the leg owes nothing or the picture is not running", () => {
    expect(owedSince(null, false, true, 1_000)).toBeNull();
    expect(owedSince(500, false, true, 1_000)).toBeNull();
    expect(owedSince(500, true, false, 1_000)).toBeNull();
  });

  it("starts the clock when a debt appears and keeps it while it lasts", () => {
    expect(owedSince(null, true, true, 1_000)).toBe(1_000);
    expect(owedSince(1_000, true, true, 5_000)).toBe(1_000);
  });

  // Prod, 2026-09-20: a viewer seeked back into audio the engine had already
  // decoded, the pump had nothing left to fetch for 32 s, and a leg playing 4
  // scheduled segments at 0 ms drift was declared starved and latched to AAC.
  it("a covered window is never a debt, however long since the last decode", () => {
    let since: number | null = null;
    for (let t = 0; t < 60_000; t += 100) since = owedSince(since, false, true, t);
    expect(isStarved({ switched: true, playing: true, owedForMs: since == null ? 0 : 60_000 - since })).toBe(false);
  });

  // Prod, 2026-09-21: an AC-3 film cut into 10-16 s segments with a 20 s
  // look-ahead window. Each segment only becomes owed when it enters the
  // window - one segment duration after the previous decode - and a decoder
  // answering instantly was declared starved on every segment over 12 s
  // (three latches that day, each predicted to 0.1 s from the playlist).
  it("a healthy leg paced by the window on long segments is never starved", () => {
    const durs = [9.973, 10.01, 9.969, 9.968, 14.77, 15.641, 8.842, 10.01, 11.97, 15.932, 12.72, 14.18];
    const starts: number[] = [];
    let acc = 0;
    for (const d of durs) { starts.push(acc); acc += d; }
    const WINDOW = 20;
    const decoded = new Set<number>();
    let since: number | null = null;
    // 100 ms ticks, decode lands one tick after the segment enters the window.
    let inflight: number | null = null;
    for (let ms = 0; ms < acc * 1000 - WINDOW * 1000; ms += 100) {
      const ct = ms / 1000;
      if (inflight != null) { decoded.add(inflight); inflight = null; since = null; }
      const pending = starts.some((s, i) => s <= ct + WINDOW && s + durs[i] > ct && !decoded.has(i));
      since = owedSince(since, pending, true, ms);
      const owedForMs = since == null ? 0 : ms - since;
      expect(isStarved({ switched: true, playing: true, owedForMs })).toBe(false);
      if (pending) inflight = starts.findIndex((s, i) => s <= ct + WINDOW && s + durs[i] > ct && !decoded.has(i));
    }
  });

  it("a dead decoder is starved once its debt is older than the threshold", () => {
    let since: number | null = null;
    let starvedAt = -1;
    for (let ms = 0; ms < 30_000; ms += 100) {
      since = owedSince(since, true, true, ms);
      if (isStarved({ switched: true, playing: true, owedForMs: since == null ? 0 : ms - since })) { starvedAt = ms; break; }
    }
    expect(starvedAt).toBe(DEFAULT_STARVATION_MS + 100);
  });
});
