import { describe, it, expect } from "vitest";
import { equatorialToHorizontal } from "../src/domain/services/horizontal";
import { localSiderealTimeDeg } from "../src/domain/services/siderealTime";

// Real J2000 coordinates (widely published, e.g. Yale Bright Star Catalogue).
const SIRIUS = { raH: 6.7525, decDeg: -16.7161 };   // dec < any mid-latitude -> transits SOUTH
const POLARIS = { raH: 2.5303, decDeg: 89.2641 };   // dec > any mid-latitude -> transits NORTH (circumpolar)
const PARIS = { latDeg: 48.8566, lonDeg: 2.3522 };

describe("equatorialToHorizontal - meridian transit invariants", () => {
  // At hour angle H=0 (LST == RA*15), altitude and azimuth are fixed by
  // spherical trigonometry alone:
  //   alt = 90 - |lat - dec|
  //   az  = 180 (south) when dec < lat, 0 (north) when dec > lat
  it("Sirius culminates due south for a northern observer", () => {
    const lst = SIRIUS.raH * 15;
    const { altDeg, azDeg } = equatorialToHorizontal(SIRIUS.raH, SIRIUS.decDeg, lst, PARIS.latDeg);
    expect(altDeg).toBeCloseTo(90 - (PARIS.latDeg - SIRIUS.decDeg), 6);
    expect(azDeg).toBeCloseTo(180, 6);
  });

  it("Polaris culminates due north for a northern observer", () => {
    const lst = POLARIS.raH * 15;
    const { altDeg, azDeg } = equatorialToHorizontal(POLARIS.raH, POLARIS.decDeg, lst, PARIS.latDeg);
    expect(altDeg).toBeCloseTo(90 - (POLARIS.decDeg - PARIS.latDeg), 6);
    expect(azDeg).toBeCloseTo(0, 6);
  });
});

describe("equatorialToHorizontal - Polaris circumpolar behavior over a real day", () => {
  it("stays within ~1.5deg of the observer's latitude and within ~8deg of due north, any time of day", () => {
    // Polaris (dec 89.2641) is 0.7359deg from the true pole, so its altitude
    // and azimuth barely wobble across a full sidereal day - a strong,
    // well-known real-world invariant that exercises the full
    // date -> sidereal time -> horizontal pipeline.
    for (let h = 0; h < 24; h += 3) {
      const ms = Date.UTC(2024, 5, 21, h, 0, 0);
      const lst = localSiderealTimeDeg(ms, PARIS.lonDeg);
      const { altDeg, azDeg } = equatorialToHorizontal(POLARIS.raH, POLARIS.decDeg, lst, PARIS.latDeg);
      expect(Math.abs(altDeg - PARIS.latDeg)).toBeLessThan(1.5);
      const azFromNorth = Math.min(azDeg, 360 - azDeg);
      expect(azFromNorth).toBeLessThan(8);
    }
  });
});

describe("equatorialToHorizontal - zenith and horizon sanity", () => {
  it("an object at dec==lat culminates at the zenith (alt=90)", () => {
    const { altDeg } = equatorialToHorizontal(0, PARIS.latDeg, 0, PARIS.latDeg);
    expect(altDeg).toBeCloseTo(90, 6);
  });
});
