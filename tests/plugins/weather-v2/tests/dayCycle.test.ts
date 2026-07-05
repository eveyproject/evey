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
});
