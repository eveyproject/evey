import { describe, it, expect } from "vitest";
import { eclipticToEquatorial } from "../src/domain/services/coordinates";

describe("eclipticToEquatorial", () => {
  it("puts an on-ecliptic point at the summer-solstice ecliptic longitude (90deg) at dec=obliquity, RA=90deg", () => {
    const obliquity = 23.4392911;
    const { raHours, decDeg } = eclipticToEquatorial(90, 0, obliquity);
    expect(decDeg).toBeCloseTo(obliquity, 6);
    expect(raHours).toBeCloseTo(6, 6); // 90deg / 15 = 6h
  });

  it("leaves the vernal equinox point (lon=0, lat=0) at RA=0, dec=0", () => {
    const { raHours, decDeg } = eclipticToEquatorial(0, 0, 23.4392911);
    expect(raHours).toBeCloseTo(0, 6);
    expect(decDeg).toBeCloseTo(0, 6);
  });

  it("a positive ecliptic latitude at the north ecliptic pole direction increases declination", () => {
    const low = eclipticToEquatorial(0, 0, 23.4392911);
    const high = eclipticToEquatorial(0, 10, 23.4392911);
    expect(high.decDeg).toBeGreaterThan(low.decDeg);
  });
});
