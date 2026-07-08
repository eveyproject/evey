// Aurora borealis/australis gating. Real data driven: only shows up at
// latitudes where the auroral oval is plausibly visible, at night, under a
// clear-enough sky - never a decorative overlay divorced from the actual
// weather/location.

import { clamp01, smoothstep } from "./dayCycle";
import type { DayCycle } from "./dayCycle";
import type { WeatherMood } from "../model/scene";

// Equatorward edge of the auroral oval under calm-to-moderate activity is
// roughly 60-65 degrees magnitude latitude; ramp in a little early (58) so
// the effect isn't an on/off cliff for cities right around the line.
const AURORA_MIN_ABS_LAT = 58;
const AURORA_RAMP_DEG = 8;

function latitudeFactor(latDeg: number): number {
  return smoothstep(AURORA_MIN_ABS_LAT, AURORA_MIN_ABS_LAT + AURORA_RAMP_DEG, Math.abs(latDeg));
}

// mood/cloudCover gate: bands only read through a mostly-clear sky. "cloudy"
// is allowed but scaled down with coverage (broken cloud still shows patches
// of aurora); anything wetter (rain/snow/fog/storm) hides it entirely.
function skyGate(mood: WeatherMood, cloudCover: number): number {
  if (mood === "clear") return 1;
  if (mood === "cloudy") return clamp01(1 - cloudCover * 2);
  return 0;
}

export function auroraIntensity(latDeg: number, cycle: DayCycle, mood: WeatherMood, cloudCover: number): number {
  const lat = latitudeFactor(latDeg);
  if (lat <= 0) return 0;
  const sky = skyGate(mood, cloudCover);
  if (sky <= 0) return 0;
  return lat * sky * cycle.night;
}
