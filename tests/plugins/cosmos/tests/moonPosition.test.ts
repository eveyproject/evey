import { describe, it, expect } from "vitest";
import { moonPosition, moonEclipticPosition } from "../src/domain/services/moonPosition";

describe("moonPosition", () => {
  it("keeps the ecliptic latitude within the Moon's real orbital inclination (~5.145deg)", () => {
    // The Moon's orbit is inclined 5.145deg to the ecliptic - a well-known
    // real fact independent of any specific date.
    for (let month = 0; month < 12; month++) {
      for (const day of [1, 10, 20]) {
        const { latDeg } = moonEclipticPosition(Date.UTC(2024, month, day, 12, 0, 0));
        expect(Math.abs(latDeg)).toBeLessThan(5.3);
      }
    }
  });

  it("produces a valid RA/dec pair (RA in [0,24), dec in [-90,90])", () => {
    const { raHours, decDeg } = moonPosition(Date.UTC(2024, 5, 15, 3, 0, 0));
    expect(raHours).toBeGreaterThanOrEqual(0);
    expect(raHours).toBeLessThan(24);
    expect(decDeg).toBeGreaterThanOrEqual(-90);
    expect(decDeg).toBeLessThanOrEqual(90);
  });

  it("the Moon completes roughly one lap of the ecliptic (~360deg) per sidereal month (~27.32 days)", () => {
    const t0 = Date.UTC(2024, 0, 1, 0, 0, 0);
    const t1 = t0 + 27.321661 * 86_400_000;
    const lon0 = moonEclipticPosition(t0).lonDeg;
    const lon1 = moonEclipticPosition(t1).lonDeg;
    const delta = ((lon1 - lon0 + 540) % 360) - 180; // signed delta in (-180,180]
    expect(Math.abs(delta)).toBeLessThan(5);
  });
});
