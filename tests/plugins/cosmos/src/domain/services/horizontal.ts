// Equatorial (RA/dec) -> horizontal (altitude/azimuth) transform.
//
// Azimuth convention: compass bearing, 0deg = North, 90deg = East, measured
// clockwise when viewed from above (the usual convention for a sky map).
// This can be proven analytically at meridian transit (hour angle H=0):
//   - if dec < lat, the object culminates due SOUTH: azimuth = 180deg
//   - if dec > lat, the object culminates due NORTH: azimuth = 0deg
// Both cases are covered by tests/horizontal.test.ts using real star
// coordinates (Sirius, Polaris).

import { toRad, toDeg, normalizeDeg, clamp } from "./mathUtils";

export interface AltAz {
  altDeg: number;
  azDeg: number;
}

export function equatorialToHorizontal(
  raHours: number,
  decDeg: number,
  lstDeg: number,
  latDeg: number,
): AltAz {
  const raDeg = raHours * 15;
  const hourAngleDeg = normalizeDeg(lstDeg - raDeg);
  const H = toRad(hourAngleDeg);
  const dec = toRad(decDeg);
  const lat = toRad(latDeg);

  const sinAlt = Math.sin(dec) * Math.sin(lat) + Math.cos(dec) * Math.cos(lat) * Math.cos(H);
  const alt = Math.asin(clamp(sinAlt, -1, 1));
  const cosAlt = Math.cos(alt);

  // Near the zenith cosAlt -> 0; azimuth is undefined there, default to 0.
  if (cosAlt < 1e-6) {
    return { altDeg: toDeg(alt), azDeg: 0 };
  }

  const cosAz = clamp((Math.sin(dec) - Math.sin(alt) * Math.sin(lat)) / (cosAlt * Math.cos(lat)), -1, 1);
  const sinAz = (-Math.sin(H) * Math.cos(dec)) / cosAlt;
  const az = Math.atan2(sinAz, cosAz);

  return { altDeg: toDeg(alt), azDeg: normalizeDeg(toDeg(az)) };
}
