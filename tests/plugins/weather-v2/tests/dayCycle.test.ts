import { describe, it, expect } from "vitest";
import { computeDayCycle } from "../src/domain/services/dayCycle";
import type { SunTimes } from "../src/domain/model/weather";

// A synthetic day: sunrise 06:00 UTC, sunset 20:00 UTC, next sunrise +24h.
const DAY0 = Date.UTC(2026, 6, 4);
const times: SunTimes = {
  sunrises: [DAY0 + 6 * 3600_000, DAY0 + 30 * 3600_000],
  sunsets: [DAY0 + 20 * 3600_000, DAY0 + 44 * 3600_000],
};

describe("computeDayCycle", () => {
  it("noon-ish: day, sun near apex", () => {
    const c = computeDayCycle(DAY0 + 13 * 3600_000, times, true);
    expect(c.isDay).toBe(true);
    expect(c.sunProgress).toBeCloseTo(0.5, 1);
    expect(c.sunY).toBeLessThan(0.35); // near widget top
    expect(c.golden).toBe(0);
    expect(c.night).toBe(0);
    expect(c.sunIntensity).toBe(1);
    expect(c.moonIntensity).toBe(0);
  });

  it("just after sunrise: golden hour, sun low left", () => {
    const c = computeDayCycle(DAY0 + 6.2 * 3600_000, times, true);
    expect(c.isDay).toBe(true);
    expect(c.golden).toBeGreaterThan(0.5);
    expect(c.sunX).toBeLessThan(0.2);
    expect(c.sunY).toBeGreaterThan(0.7); // near horizon
  });

  it("just before sunset: golden hour, sun low right", () => {
    const c = computeDayCycle(DAY0 + 19.8 * 3600_000, times, true);
    expect(c.isDay).toBe(true);
    expect(c.golden).toBeGreaterThan(0.5);
    expect(c.sunX).toBeGreaterThan(0.8);
  });

  it("midnight: night, moon mid-arc", () => {
    // Night window 20:00 -> 30:00 (10h), 01:00 is progress 0.5.
    const c = computeDayCycle(DAY0 + 25 * 3600_000, times, false);
    expect(c.isDay).toBe(false);
    expect(c.moonProgress).toBeCloseTo(0.5, 1);
    expect(c.night).toBeGreaterThan(0.8);
    expect(c.sunIntensity).toBe(0);
    expect(c.moonIntensity).toBe(1);
  });

  it("pre-dawn uses yesterday's sunset bracket", () => {
    // 04:00 on day1 = still inside the same night window.
    const c = computeDayCycle(DAY0 + 28 * 3600_000, times, false);
    expect(c.isDay).toBe(false);
    expect(c.moonProgress).toBeGreaterThan(0.7);
  });

  it("positions stay inside the widget", () => {
    for (let h = 0; h < 48; h += 0.5) {
      const c = computeDayCycle(DAY0 + h * 3600_000, times, true);
      for (const v of [c.sunX, c.sunY, c.moonX, c.moonY]) {
        expect(v).toBeGreaterThanOrEqual(0);
        expect(v).toBeLessThanOrEqual(1);
      }
      expect(c.golden).toBeGreaterThanOrEqual(0);
      expect(c.golden).toBeLessThanOrEqual(1);
    }
  });

  it("null sun times falls back without throwing", () => {
    const c = computeDayCycle(Date.UTC(2026, 6, 4, 12), null, true);
    expect(c.isDay).toBe(true);
    const n = computeDayCycle(Date.UTC(2026, 6, 4, 23), null, false);
    expect(n.isDay).toBe(false);
  });

  it("empty arrays fall back", () => {
    const c = computeDayCycle(DAY0, { sunrises: [], sunsets: [] }, true);
    expect(c.isDay).toBe(true);
  });

  // Polar/high-latitude regression: a "day" or "night" bracket can legitimately
  // span many hours (or days, near the poles). The golden-hour/twilight glow
  // must stay anchored to a fixed real-time window at the terminator instead
  // of stretching in proportion to the whole day/night length.
  it("golden-hour width is a fixed real-time budget, not a fraction of day length", () => {
    const shortDay: SunTimes = {
      sunrises: [DAY0 + 6 * 3600_000],
      sunsets: [DAY0 + 18 * 3600_000], // 12h day
    };
    const polarDay: SunTimes = {
      sunrises: [DAY0 + 6 * 3600_000],
      sunsets: [DAY0 + 6 * 3600_000 + 60 * 24 * 3600_000], // 60-day polar-summer "day"
    };
    const oneHourAfterRise = DAY0 + 7 * 3600_000;
    const shortDayGolden = computeDayCycle(oneHourAfterRise, shortDay, true).golden;
    const polarDayGolden = computeDayCycle(oneHourAfterRise, polarDay, true).golden;
    // Same real elapsed time since sunrise (1h) -> same golden-hour falloff,
    // regardless of how long the overall "day" bracket is.
    expect(polarDayGolden).toBeCloseTo(shortDayGolden, 5);
    // A full day into a 60-day polar day, the old fraction-based formula would
    // still report ~0 golden (1/60th of the way in); confirm it's actually 0
    // now that the window is a fixed 45 minutes.
    const oneDayIn = computeDayCycle(DAY0 + 6 * 3600_000 + 24 * 3600_000, polarDay, true);
    expect(oneDayIn.golden).toBe(0);
  });

  it("twilight width at night follows the same fixed real-time rule", () => {
    const shortNight: SunTimes = {
      sunrises: [DAY0 + 30 * 3600_000],
      sunsets: [DAY0 + 20 * 3600_000], // 10h night
    };
    const polarNight: SunTimes = {
      sunrises: [DAY0 + 6 * 3600_000 + 60 * 24 * 3600_000],
      sunsets: [DAY0 + 20 * 3600_000], // 59+ day polar-winter "night"
    };
    const oneHourAfterSet = DAY0 + 21 * 3600_000;
    const shortNightGolden = computeDayCycle(oneHourAfterSet, shortNight, false).golden;
    const polarNightGolden = computeDayCycle(oneHourAfterSet, polarNight, false).golden;
    expect(polarNightGolden).toBeCloseTo(shortNightGolden, 5);
  });

  // Degenerate/corrupt provider data (identical or near-identical sunrise and
  // sunset timestamps) must never propagate a division-by-zero into the scene.
  it("near-identical sunrise/sunset never produces non-finite output", () => {
    const degenerate: SunTimes = {
      sunrises: [DAY0 + 6 * 3600_000],
      sunsets: [DAY0 + 6 * 3600_000], // zero-length "day"
    };
    const c = computeDayCycle(DAY0 + 6 * 3600_000, degenerate, true);
    for (const v of [c.sunX, c.sunY, c.moonX, c.moonY, c.golden, c.night]) {
      expect(Number.isFinite(v)).toBe(true);
    }
  });
});
