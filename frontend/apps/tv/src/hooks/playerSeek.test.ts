import { describe, it, expect } from "vitest";
import { seekDeltaForKey, clampSeekTarget, shouldReleaseSeekPreview, SEEK_ACCELERATION_MULTIPLIER } from "./playerSeek";

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
