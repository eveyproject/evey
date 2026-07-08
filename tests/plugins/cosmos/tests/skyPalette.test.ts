import { describe, it, expect } from "vitest";
import { starVisibility, twilightWarmth, skyMood } from "../src/domain/services/skyPalette";

describe("starVisibility", () => {
  it("is ~0 in broad daylight and ~1 in deep night", () => {
    expect(starVisibility(45)).toBeCloseTo(0, 3);
    expect(starVisibility(-40)).toBeCloseTo(1, 3);
  });

  it("is monotonically non-decreasing as the Sun sinks", () => {
    const samples = [30, 10, 0, -6, -12, -18, -30].map(starVisibility);
    for (let i = 1; i < samples.length; i++) {
      expect(samples[i]).toBeGreaterThanOrEqual(samples[i - 1]);
    }
  });
});

describe("twilightWarmth", () => {
  it("peaks near the horizon and fades away in day and deep night", () => {
    const atHorizon = twilightWarmth(0);
    const atNoon = twilightWarmth(60);
    const atNight = twilightWarmth(-40);
    expect(atHorizon).toBeGreaterThan(atNoon);
    expect(atHorizon).toBeGreaterThan(atNight);
  });
});

describe("skyMood", () => {
  it("classifies day/twilight/night from the real twilight thresholds", () => {
    expect(skyMood(20)).toBe("day");
    expect(skyMood(-8)).toBe("twilight");
    expect(skyMood(-20)).toBe("night");
  });
});
