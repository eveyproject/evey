import { describe, it, expect } from "vitest";
import { greenwichMeanSiderealTimeDeg, localSiderealTimeDeg } from "../src/domain/services/siderealTime";

describe("greenwichMeanSiderealTimeDeg", () => {
  it("equals the documented GMST at J2000.0 (2000-01-01 12:00 UT)", () => {
    // Well-known reference fact: GMST at the J2000.0 epoch is 280.46061837 deg
    // (18h 41m 50.5s), which is also the constant term of the IAU formula.
    const ms = Date.UTC(2000, 0, 1, 12, 0, 0);
    expect(greenwichMeanSiderealTimeDeg(ms)).toBeCloseTo(280.46061837, 4);
  });

  it("advances by roughly 360.9856 deg per solar day (one sidereal day is ~4 minutes shorter)", () => {
    const t0 = Date.UTC(2024, 5, 1, 0, 0, 0);
    const t1 = t0 + 86_400_000;
    const g0 = greenwichMeanSiderealTimeDeg(t0);
    const g1 = greenwichMeanSiderealTimeDeg(t1);
    const delta = ((g1 - g0 + 360) % 360);
    expect(delta).toBeCloseTo(0.9856, 2);
  });

  it("stays within [0, 360)", () => {
    for (let h = 0; h < 24; h++) {
      const g = greenwichMeanSiderealTimeDeg(Date.UTC(2025, 2, 15, h, 0, 0));
      expect(g).toBeGreaterThanOrEqual(0);
      expect(g).toBeLessThan(360);
    }
  });
});

describe("localSiderealTimeDeg", () => {
  it("equals GMST plus east longitude", () => {
    const ms = Date.UTC(2024, 5, 1, 0, 0, 0);
    const gmst = greenwichMeanSiderealTimeDeg(ms);
    expect(localSiderealTimeDeg(ms, 10)).toBeCloseTo((gmst + 10) % 360, 6);
  });

  it("wraps west longitudes correctly", () => {
    const ms = Date.UTC(2024, 5, 1, 0, 0, 0);
    const lst = localSiderealTimeDeg(ms, -170);
    expect(lst).toBeGreaterThanOrEqual(0);
    expect(lst).toBeLessThan(360);
  });
});
