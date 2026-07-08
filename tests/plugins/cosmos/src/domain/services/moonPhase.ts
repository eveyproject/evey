// Moon phase from the true Sun-Moon ecliptic elongation (geometrically
// accurate, ties directly to moonPosition.ts/sunPosition.ts), cross-checked
// in tests against the classic synodic-period estimate (the same simple model
// used by the weather-v2 plugin) - two independent methods for the same
// physical quantity should agree closely.

import { normalizeDeg, toRad } from "./mathUtils";

export type PhaseKey =
  | "new"
  | "waxing_crescent"
  | "first_quarter"
  | "waxing_gibbous"
  | "full"
  | "waning_gibbous"
  | "last_quarter"
  | "waning_crescent";

export interface MoonPhase {
  elongationDeg: number;      // 0 = new, 180 = full
  illuminatedFraction: number; // 0..1
  waxing: boolean;
  phaseKey: PhaseKey;
}

// Reference synodic-cycle estimate (age since a known new moon), used only as
// an independent cross-check in tests - not for the actual phase computation.
const REF_NEW_MOON_MS = Date.UTC(2000, 0, 6, 18, 14, 0);
const SYNODIC_DAYS = 29.530588853;
const SYNODIC_MS = SYNODIC_DAYS * 86_400_000;

export function synodicAgeFraction(nowMs: number): number {
  const p = ((nowMs - REF_NEW_MOON_MS) % SYNODIC_MS) / SYNODIC_MS;
  return p < 0 ? p + 1 : p;
}

function phaseKeyFromElongation(elong: number): PhaseKey {
  if (elong < 22.5) return "new";
  if (elong < 67.5) return "waxing_crescent";
  if (elong < 112.5) return "first_quarter";
  if (elong < 157.5) return "waxing_gibbous";
  if (elong < 202.5) return "full";
  if (elong < 247.5) return "waning_gibbous";
  if (elong < 292.5) return "last_quarter";
  if (elong < 337.5) return "waning_crescent";
  return "new";
}

export function moonPhaseFromElongation(moonLonDeg: number, sunLonDeg: number): MoonPhase {
  const elong = normalizeDeg(moonLonDeg - sunLonDeg); // 0..360, moon east of sun as it waxes
  const illuminatedFraction = (1 - Math.cos(toRad(elong))) / 2;
  return {
    elongationDeg: elong,
    illuminatedFraction,
    waxing: elong < 180,
    phaseKey: phaseKeyFromElongation(elong),
  };
}

// Days remaining until the next full moon, derived from the current
// elongation (0 at new, 180 at full) and the synodic period.
export function daysToFullMoon(elongationDeg: number): number {
  const degToFull = normalizeDeg(180 - elongationDeg);
  return (degToFull / 360) * SYNODIC_DAYS;
}
