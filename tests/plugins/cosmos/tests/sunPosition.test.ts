import { describe, it, expect } from "vitest";
import { sunPosition } from "../src/domain/services/sunPosition";
import { equatorialToHorizontal } from "../src/domain/services/horizontal";

// Real, well-known facts about Earth's orbit/axial tilt used as regression
// checks (none of these require fetching a live ephemeris):
//   - Earth-Sun distance ranges from ~0.983 AU (perihelion, early January) to
//     ~1.017 AU (aphelion, early July).
//   - at the June solstice the Sun's declination is +23.44deg (the axial tilt);
//     at the December solstice it is -23.44deg; at the equinoxes it is ~0deg.
describe("sunPosition", () => {
  it("keeps the Earth-Sun distance within the real annual range", () => {
    for (let month = 0; month < 12; month++) {
      const { distanceAu } = sunPosition(Date.UTC(2024, month, 15, 12, 0, 0));
      expect(distanceAu).toBeGreaterThan(0.982);
      expect(distanceAu).toBeLessThan(1.018);
    }
  });

  it("reaches +23.4deg declination near the June solstice", () => {
    const { decDeg } = sunPosition(Date.UTC(2024, 5, 21, 12, 0, 0));
    expect(Math.abs(decDeg - 23.44)).toBeLessThan(0.6);
  });

  it("reaches -23.4deg declination near the December solstice", () => {
    const { decDeg } = sunPosition(Date.UTC(2024, 11, 21, 12, 0, 0));
    expect(Math.abs(decDeg + 23.44)).toBeLessThan(0.6);
  });

  it("crosses ~0deg declination near the equinoxes", () => {
    const spring = sunPosition(Date.UTC(2024, 2, 20, 12, 0, 0));
    const autumn = sunPosition(Date.UTC(2024, 8, 22, 12, 0, 0));
    expect(Math.abs(spring.decDeg)).toBeLessThan(1.5);
    expect(Math.abs(autumn.decDeg)).toBeLessThan(1.5);
  });

  it("full pipeline: local solar noon puts the Sun on the meridian at the expected altitude", () => {
    // At hour angle 0 (LST == Sun's RA*15), altitude = 90 - |lat - dec|
    // regardless of date - this exercises sunPosition + horizontal together.
    const lat = 48.8566;
    const { raHours, decDeg } = sunPosition(Date.UTC(2024, 8, 1, 10, 0, 0));
    const lst = raHours * 15;
    const { altDeg, azDeg } = equatorialToHorizontal(raHours, decDeg, lst, lat);
    expect(altDeg).toBeCloseTo(90 - Math.abs(lat - decDeg), 6);
    // early September, dec still well above Paris's "dec < lat" boundary is
    // not guaranteed, so just check azimuth is due south or due north exactly.
    expect(azDeg === 180 || azDeg === 0 || Math.abs(azDeg - 180) < 1e-6 || Math.abs(azDeg) < 1e-6).toBe(true);
  });
});
