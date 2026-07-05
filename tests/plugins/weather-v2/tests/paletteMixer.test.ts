import { describe, it, expect } from "vitest";
import { mixPalette } from "../src/domain/services/paletteMixer";
import { computeDayCycle } from "../src/domain/services/dayCycle";
import type { SunTimes } from "../src/domain/model/weather";
import type { WeatherMood } from "../src/domain/model/scene";

const DAY0 = Date.UTC(2026, 6, 4);
const times: SunTimes = {
  sunrises: [DAY0 + 6 * 3600_000, DAY0 + 30 * 3600_000],
  sunsets: [DAY0 + 20 * 3600_000, DAY0 + 44 * 3600_000],
};
const noon = computeDayCycle(DAY0 + 13 * 3600_000, times, true);
const midnight = computeDayCycle(DAY0 + 25 * 3600_000, times, false);
const dusk = computeDayCycle(DAY0 + 19.9 * 3600_000, times, true);

const MOODS: WeatherMood[] = ["clear", "cloudy", "rain", "snow", "fog", "storm"];

function lum(c: [number, number, number]): number {
  return c[0] * 0.299 + c[1] * 0.587 + c[2] * 0.114;
}

describe("mixPalette", () => {
  it("all moods x all times produce finite 0..~1.2 channels", () => {
    for (const mood of MOODS) {
      for (const cycle of [noon, midnight, dusk]) {
        const p = mixPalette(mood, cycle);
        for (const c of [p.cloudColor, p.cloudShadow, p.sunlight, p.fogColor, p.glowColor]) {
          for (const v of c) {
            expect(Number.isFinite(v)).toBe(true);
            expect(v).toBeGreaterThanOrEqual(0);
            expect(v).toBeLessThanOrEqual(1.3);
          }
        }
      }
    }
  });

  it("storm clouds are darker than clear clouds", () => {
    expect(lum(mixPalette("storm", noon).cloudColor)).toBeLessThan(lum(mixPalette("clear", noon).cloudColor));
  });

  it("night clouds are darker than day clouds", () => {
    expect(lum(mixPalette("clear", midnight).cloudColor)).toBeLessThan(lum(mixPalette("clear", noon).cloudColor));
  });

  it("golden hour warms the sunlight (more red than blue)", () => {
    const p = mixPalette("clear", dusk);
    expect(p.sunlight[0]).toBeGreaterThan(p.sunlight[2]);
  });

  it("glow vanishes at deep night", () => {
    const p = mixPalette("clear", midnight);
    expect(lum(p.glowColor)).toBeLessThan(0.35);
  });
});
