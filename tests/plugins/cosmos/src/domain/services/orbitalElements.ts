// Low-precision Keplerian orbital elements and the two-body position solver
// (Paul Schlyter's widely reproduced "How to compute planetary positions"
// method - degree-level accuracy, which is all a sky-widget needs). The Sun
// row below is really Earth's own orbit (N=i=0), so the same solver produces
// both the Sun's apparent geocentric position and each planet's heliocentric
// position; planetPositions.ts combines the two into a geocentric position.
//
// Elements are linear in `d` = days since 2000 Jan 0.0 UT (see
// time.daysSinceElementsEpoch), matching the epoch these mean-element tables
// are quoted against.

import { toRad, toDeg, normalizeDeg } from "./mathUtils";

export interface OrbitalElements {
  N0: number; Nd: number; // longitude of ascending node (deg, deg/day)
  i0: number; id: number; // inclination (deg, deg/day)
  w0: number; wd: number; // argument of perihelion (deg, deg/day)
  a: number;               // semi-major axis (AU) - treated as constant
  e0: number; ed: number; // eccentricity (dimensionless, /day)
  M0: number; Md: number; // mean anomaly (deg, deg/day)
}

export const SUN_ELEMENTS: OrbitalElements = {
  // N/i are meaningless for the reference plane itself; kept at 0.
  N0: 0, Nd: 0,
  i0: 0, id: 0,
  w0: 282.9404, wd: 4.70935e-5,
  a: 1.0,
  e0: 0.016709, ed: -1.151e-9,
  M0: 356.047, Md: 0.9856002585,
};

export const VENUS_ELEMENTS: OrbitalElements = {
  N0: 76.6799, Nd: 2.4659e-5,
  i0: 3.3946, id: 2.75e-8,
  w0: 54.891, wd: 1.38374e-5,
  a: 0.72333,
  e0: 0.006773, ed: -1.302e-9,
  M0: 48.0052, Md: 1.6021302244,
};

export const MARS_ELEMENTS: OrbitalElements = {
  N0: 49.5574, Nd: 2.11081e-5,
  i0: 1.8497, id: -1.78e-8,
  w0: 286.5016, wd: 2.92961e-5,
  a: 1.523688,
  e0: 0.093405, ed: 2.516e-9,
  M0: 18.6021, Md: 0.5240207766,
};

export const JUPITER_ELEMENTS: OrbitalElements = {
  N0: 100.4542, Nd: 2.76854e-5,
  i0: 1.303, id: -1.557e-7,
  w0: 273.8777, wd: 1.64505e-5,
  a: 5.20256,
  e0: 0.048498, ed: 4.469e-9,
  M0: 19.895, Md: 0.0830853001,
};

export const SATURN_ELEMENTS: OrbitalElements = {
  N0: 113.6634, Nd: 2.3898e-5,
  i0: 2.4886, id: -1.081e-7,
  w0: 339.3939, wd: 2.97661e-5,
  a: 9.55475,
  e0: 0.055546, ed: -9.499e-9,
  M0: 316.967, Md: 0.0334442282,
};

export interface HeliocentricPosition {
  xh: number;
  yh: number;
  zh: number;
  distanceAu: number; // heliocentric distance
  trueLonDeg: number; // v + w, useful for the Sun row directly
}

// Newton-Raphson solve of Kepler's equation E - e*sin(E) = M, all in degrees.
function solveEccentricAnomalyDeg(meanAnomalyDeg: number, e: number): number {
  const M = meanAnomalyDeg;
  let E = M + toDeg(e * Math.sin(toRad(M)) * (1 + e * Math.cos(toRad(M))));
  for (let i = 0; i < 8; i++) {
    const Erad = toRad(E);
    const delta = (E - toDeg(e * Math.sin(Erad)) - M) / (1 - e * Math.cos(Erad));
    E -= delta;
    if (Math.abs(delta) < 1e-9) break;
  }
  return E;
}

export function heliocentricPosition(el: OrbitalElements, d: number): HeliocentricPosition {
  const N = toRad(normalizeDeg(el.N0 + el.Nd * d));
  const i = toRad(el.i0 + el.id * d);
  const w = toRad(normalizeDeg(el.w0 + el.wd * d));
  const a = el.a;
  const e = el.e0 + el.ed * d;
  const M = normalizeDeg(el.M0 + el.Md * d);

  const E = toRad(solveEccentricAnomalyDeg(M, e));

  const xv = a * (Math.cos(E) - e);
  const yv = a * (Math.sqrt(1 - e * e) * Math.sin(E));
  const v = Math.atan2(yv, xv);
  const r = Math.sqrt(xv * xv + yv * yv);

  const vw = v + w;
  const xh = r * (Math.cos(N) * Math.cos(vw) - Math.sin(N) * Math.sin(vw) * Math.cos(i));
  const yh = r * (Math.sin(N) * Math.cos(vw) + Math.cos(N) * Math.sin(vw) * Math.cos(i));
  const zh = r * (Math.sin(vw) * Math.sin(i));

  return { xh, yh, zh, distanceAu: r, trueLonDeg: normalizeDeg(toDeg(vw)) };
}
