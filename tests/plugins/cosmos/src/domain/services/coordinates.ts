// Ecliptic (longitude/latitude) -> equatorial (RA/dec) conversion, shared by
// the Sun, Moon and planet position models. Standard spherical-astronomy
// formula (e.g. Meeus ch. 13): reduces to the textbook single-angle case at
// latitude=0 (an ecliptic-longitude-only point at the summer-solstice point,
// lambda=90, lands exactly at dec=obliquity, RA=90 - see
// tests/coordinates.test.ts).

import { toRad, toDeg, normalizeDeg, clamp } from "./mathUtils";

export interface EquatorialCoord {
  raHours: number;
  decDeg: number;
}

export function eclipticToEquatorial(lonDeg: number, latDeg: number, obliquityDeg: number): EquatorialCoord {
  const lon = toRad(lonDeg);
  const lat = toRad(latDeg);
  const eps = toRad(obliquityDeg);

  const sinDec = Math.sin(lat) * Math.cos(eps) + Math.cos(lat) * Math.sin(eps) * Math.sin(lon);
  const decDeg = toDeg(Math.asin(clamp(sinDec, -1, 1)));

  const y = Math.sin(lon) * Math.cos(eps) - Math.tan(lat) * Math.sin(eps);
  const x = Math.cos(lon);
  const raDeg = normalizeDeg(toDeg(Math.atan2(y, x)));

  return { raHours: raDeg / 15, decDeg };
}
