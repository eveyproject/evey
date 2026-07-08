// Moon position via Meeus's abbreviated lunar theory (the truncated leading
// terms of ELP2000, accurate to a few arcminutes in longitude/latitude - far
// tighter than the "degree precision" this widget needs). This is the same
// series reproduced in most "compute the Moon's position" references.

import { toRad, normalizeDeg } from "./mathUtils";
import { eclipticToEquatorial, type EquatorialCoord } from "./coordinates";
import { julianCenturiesJ2000, meanObliquityDeg } from "./time";

export interface MoonPosition extends EquatorialCoord {
  eclipticLonDeg: number;
  eclipticLatDeg: number;
}

export function moonEclipticPosition(nowMs: number): { lonDeg: number; latDeg: number } {
  const T = julianCenturiesJ2000(nowMs);

  const Lp = normalizeDeg(218.3164477 + 481267.88123421 * T); // mean longitude
  const D = normalizeDeg(297.8501921 + 445267.1114034 * T);   // mean elongation from the Sun
  const M = normalizeDeg(357.5291092 + 35999.0502909 * T);    // Sun's mean anomaly
  const Mp = normalizeDeg(134.9633964 + 477198.8675055 * T);  // Moon's mean anomaly
  const F = normalizeDeg(93.272095 + 483202.0175233 * T);     // Moon's argument of latitude

  const d = toRad(D), m = toRad(M), mp = toRad(Mp), f = toRad(F);

  const lonCorrection =
    6.289 * Math.sin(mp) -
    1.274 * Math.sin(mp - 2 * d) +
    0.658 * Math.sin(2 * d) -
    0.186 * Math.sin(m) -
    0.059 * Math.sin(2 * mp - 2 * d) -
    0.057 * Math.sin(mp - 2 * d + m) +
    0.053 * Math.sin(mp + 2 * d) +
    0.046 * Math.sin(2 * d - m) +
    0.041 * Math.sin(mp - m) -
    0.035 * Math.sin(d) -
    0.031 * Math.sin(mp + m) -
    0.015 * Math.sin(2 * f - 2 * d) +
    0.011 * Math.sin(mp - 4 * d);

  const latCorrection =
    5.128 * Math.sin(f) +
    0.281 * Math.sin(mp + f) +
    0.278 * Math.sin(mp - f) +
    0.173 * Math.sin(2 * d - f) +
    0.055 * Math.sin(2 * d - mp + f) +
    0.046 * Math.sin(2 * d - mp - f) +
    0.033 * Math.sin(2 * d + f) +
    0.017 * Math.sin(2 * mp + f);

  return {
    lonDeg: normalizeDeg(Lp + lonCorrection),
    latDeg: latCorrection,
  };
}

export function moonPosition(nowMs: number): MoonPosition {
  const { lonDeg, latDeg } = moonEclipticPosition(nowMs);
  const equatorial = eclipticToEquatorial(lonDeg, latDeg, meanObliquityDeg(nowMs));
  return { ...equatorial, eclipticLonDeg: lonDeg, eclipticLatDeg: latDeg };
}
