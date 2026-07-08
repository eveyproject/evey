import { describe, it, expect } from "vitest";
import { auroraIntensity } from "../src/domain/services/aurora";
import { computeDayCycle } from "../src/domain/services/dayCycle";
import type { SunTimes } from "../src/domain/model/weather";

const DAY0 = Date.UTC(2026, 6, 4);
const times: SunTimes = {
  sunrises: [DAY0 + 6 * 3600_000, DAY0 + 30 * 3600_000],
  sunsets: [DAY0 + 20 * 3600_000, DAY0 + 44 * 3600_000],
};
const noon = computeDayCycle(DAY0 + 13 * 3600_000, times, true);
const midnight = computeDayCycle(DAY0 + 25 * 3600_000, times, false);

describe("auroraIntensity", () => {
  it("mid latitudes (Paris ~49N) never show aurora, day or night", () => {
    expect(auroraIntensity(48.85, noon, "clear", 0)).toBe(0);
    expect(auroraIntensity(48.85, midnight, "clear", 0)).toBe(0);
  });

  it("high latitude (Reykjavik ~64N) shows aurora at clear deep night", () => {
    expect(auroraIntensity(64.1, midnight, "clear", 0)).toBeGreaterThan(0);
  });

  it("high latitude during the day is zero (cycle.night gates it)", () => {
    expect(auroraIntensity(64.1, noon, "clear", 0)).toBe(0);
  });

  it("southern high latitude works via absolute value", () => {
    expect(auroraIntensity(-66, midnight, "clear", 0)).toBeGreaterThan(0);
  });

  it("wet weather (rain/snow/fog/storm) always hides it", () => {
    for (const mood of ["rain", "snow", "fog", "storm"] as const) {
      expect(auroraIntensity(70, midnight, mood, 0)).toBe(0);
    }
  });

  it("overcast hides it, light cloud only dims it", () => {
    expect(auroraIntensity(70, midnight, "cloudy", 0.9)).toBe(0);
    const lightCloud = auroraIntensity(70, midnight, "cloudy", 0.1);
    const clear = auroraIntensity(70, midnight, "clear", 0);
    expect(lightCloud).toBeGreaterThan(0);
    expect(lightCloud).toBeLessThan(clear);
  });

  it("intensity ramps up with latitude past the auroral-oval edge", () => {
    const near = auroraIntensity(59, midnight, "clear", 0);
    const far = auroraIntensity(75, midnight, "clear", 0);
    expect(far).toBeGreaterThan(near);
  });

  it("always returns a finite value in [0, 1]", () => {
    for (const lat of [-90, -66, -20, 0, 20, 66, 90]) {
      for (const mood of ["clear", "cloudy", "rain", "snow", "fog", "storm"] as const) {
        for (const cycle of [noon, midnight]) {
          const v = auroraIntensity(lat, cycle, mood, 0.5);
          expect(Number.isFinite(v)).toBe(true);
          expect(v).toBeGreaterThanOrEqual(0);
          expect(v).toBeLessThanOrEqual(1);
        }
      }
    }
  });
});
