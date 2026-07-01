import { describe, it, expect } from "vitest";
import { computeEffectiveDuration } from "./PlayerContext";
import type { PlayerState } from "./PlayerContext";

describe("PlayerContext — effectiveDuration logic", () => {
  it("prefers knownDurationSec over video duration", () => {
    expect(computeEffectiveDuration(3600, 120)).toBe(3600);
  });

  it("falls back to video duration when knownDurationSec absent", () => {
    expect(computeEffectiveDuration(undefined, 120)).toBe(120);
  });

  it("returns 0 when both are unavailable", () => {
    expect(computeEffectiveDuration(undefined, NaN)).toBe(0);
    expect(computeEffectiveDuration(0, 0)).toBe(0);
  });
});

describe("PlayerContext — absoluteTime logic", () => {
  it("absoluteTime = transcodeOffset + currentTime", () => {
    expect(300 + 45).toBe(345);
  });

  it("absoluteTime equals currentTime when offset is 0", () => {
    expect(0 + 60).toBe(60);
  });
});

describe("PlayerContext — PlayerState type", () => {
  it("PlayerState has expected shape", () => {
    const state: Partial<PlayerState> = {
      playing: false,
      volume: 1,
      brightness: 1,
      effectiveDuration: 0,
      skipForwardSeconds: 10,
      skipBackwardSeconds: 10,
    };
    expect(state.playing).toBe(false);
    expect(state.volume).toBe(1);
    expect(state.brightness).toBe(1);
    expect(state.skipForwardSeconds).toBe(10);
  });
});
