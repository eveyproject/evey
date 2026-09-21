import { describe, it, expect } from "vitest";
import { seekDeltaForKey, clampSeekTarget, shouldReleaseSeekPreview, SEEK_ACCELERATION_MULTIPLIER, tapSeekStep, holdScrubRate, TAP_SEEK_MAX_MULTIPLIER } from "./seekGesture";

describe("seekDeltaForKey", () => {
  it("moves backward by the plain skip amount on left", () => {
    expect(seekDeltaForKey("left", 10, 30)).toBe(-10);
  });

  it("moves forward by the plain skip amount on right", () => {
    expect(seekDeltaForKey("right", 10, 30)).toBe(30);
  });

  it("accelerates backward on a held left (longLeft)", () => {
    expect(seekDeltaForKey("longLeft", 10, 30)).toBe(-10 * SEEK_ACCELERATION_MULTIPLIER);
  });

  it("accelerates forward on a held right (longRight)", () => {
    expect(seekDeltaForKey("longRight", 10, 30)).toBe(30 * SEEK_ACCELERATION_MULTIPLIER);
  });

  it("is a no-op for any other key", () => {
    expect(seekDeltaForKey("up", 10, 30)).toBe(0);
    expect(seekDeltaForKey("select", 10, 30)).toBe(0);
  });
});

describe("clampSeekTarget", () => {
  it("clamps to 0 at the start of the timeline", () => {
    expect(clampSeekTarget(5, -20, 100)).toBe(0);
  });

  it("clamps to duration at the end of the timeline", () => {
    expect(clampSeekTarget(95, 20, 100)).toBe(100);
  });

  it("passes through untouched within range", () => {
    expect(clampSeekTarget(50, 10, 100)).toBe(60);
  });

  it("does not clamp upward when duration is not known yet", () => {
    expect(clampSeekTarget(50, 1000, 0)).toBe(1050);
  });
});

describe("shouldReleaseSeekPreview", () => {
  it("releases once the real position is within tolerance", () => {
    expect(shouldReleaseSeekPreview(100, 101)).toBe(true);
    expect(shouldReleaseSeekPreview(100, 98)).toBe(true);
  });

  it("holds the preview while the real position is still catching up", () => {
    expect(shouldReleaseSeekPreview(90, 100)).toBe(false);
  });

  it("respects a custom tolerance", () => {
    expect(shouldReleaseSeekPreview(90, 100, 20)).toBe(true);
  });
});

// Twenty taps of thirty seconds to reach ten minutes read as "stuck at
// thirty seconds" (owner, 2026-09-22). The step grows every second tap.
describe("tapSeekStep", () => {
  it("keeps the first two taps at the plain skip, then doubles every second tap", () => {
    expect([1, 2, 3, 4, 5, 6, 7, 8].map((n) => tapSeekStep(n, 30))).toEqual([30, 30, 60, 60, 120, 120, 240, 240]);
  });
  it("caps the growth so a long run of taps stays predictable", () => {
    expect(tapSeekStep(40, 30)).toBe(30 * TAP_SEEK_MAX_MULTIPLIER);
  });
  it("treats a zero or negative count as the first tap", () => {
    expect(tapSeekStep(0, 10)).toBe(10);
  });
});

describe("holdScrubRate", () => {
  it("scrubs a minute per second at first, then four, then ten", () => {
    expect(holdScrubRate(0)).toBe(60);
    expect(holdScrubRate(999)).toBe(60);
    expect(holdScrubRate(1000)).toBe(240);
    expect(holdScrubRate(2999)).toBe(240);
    expect(holdScrubRate(3000)).toBe(600);
  });
  it("crosses a two-hour film in about fifteen seconds of holding", () => {
    let pos = 0;
    for (let ms = 0; ms < 15_000; ms += 100) pos += holdScrubRate(ms) * 0.1;
    expect(pos).toBeGreaterThan(7200);
  });
});
