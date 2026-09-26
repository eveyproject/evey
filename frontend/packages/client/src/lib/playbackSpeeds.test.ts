import { describe, it, expect } from "vitest";
import { nextPlaybackSpeed, SPEED_OPTIONS } from "./playbackSpeeds";

describe("playbackSpeeds", () => {
  it("steps up and wraps round to the slowest", () => {
    expect(nextPlaybackSpeed(1)).toBe(1.25);
    expect(nextPlaybackSpeed(2)).toBe(0.5);
    expect(SPEED_OPTIONS[0]).toBe(0.5);
  });

  it("starts from the slowest for a speed off the list", () => {
    expect(nextPlaybackSpeed(1.1)).toBe(0.5);
  });
});
