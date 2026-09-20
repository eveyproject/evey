// Day/night cycle derived from REAL sunrise/sunset events (Open-Meteo daily
// API), so the sun position is correct at any latitude and season. This
// replaces v1's hour-of-day sine approximation, which had no latitude or
// season correction and needed runtime hacks to keep the sun in frame.
//
// Widget coordinates: x and y in 0..1, y=0 at the TOP of the widget.
// The sun travels a flat arc: rises at x~0.08, culminates near the top
// center, sets at x~0.92. The moon travels the same arc during the night
// window (sunset -> next sunrise).

import type { SunTimes } from "../model/weather";

export interface DayCycle {
  isDay: boolean;
  sunProgress: number;   // 0 at sunrise .. 1 at sunset (0 when night)
  moonProgress: number;  // 0 at sunset .. 1 at next sunrise (0 when day)
  golden: number;        // 0..1, peaks at sunrise/sunset (warm palette)
  night: number;         // 0..1, 1 in deep night
  sunX: number;
  sunY: number;
  sunIntensity: number;  // 0..1 target visibility
  moonX: number;
  moonY: number;
  moonIntensity: number;
}

export function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

export function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = clamp01((x - edge0) / (edge1 - edge0));
  return t * t * (3 - 2 * t);
}

// Real-world width of the golden-hour / twilight wash, in milliseconds. This
// must NOT scale with the length of the day/night: at high latitudes a
// "day" or "night" can last many hours (even weeks near the poles), and
// sizing the warm-light window as a FRACTION of that duration (the v2.8.0
// behaviour) stretched sunrise/sunset colours across most of the visible day
// - clearly wrong. Anchoring the window to a fixed real-time budget keeps the
// glow at the terminator regardless of season or latitude.
const GOLDEN_WINDOW_MS = 45 * 60 * 1000;
const TWILIGHT_WINDOW_MS = 40 * 60 * 1000;
const FALLBACK_HALF_DAY_MS = 12 * 3600_000;
// Sunrise/sunset pairs closer together than this are treated as unusable
// (guards the (set - rise) division below against NaN/Infinity on
// degenerate/duplicate provider data instead of ever handing the renderer a
// non-finite sun/moon position).
const MIN_EVENT_GAP_MS = 60_000;

// Arc across the widget: progress 0..1 -> (x, y) with y=0 at top.
function arcPosition(progress: number): { x: number; y: number } {
  const p = clamp01(progress);
  return {
    x: 0.08 + 0.84 * p,
    // horizon-ish at 0.85, apex at 0.33: the sun disc is 0.29 of the height
    // (shaders.ts SR), so a higher apex clips it at noon - it did, at 0.22.
    y: 0.85 - 0.52 * Math.sin(Math.PI * p),
  };
}

// Fallback when sunrise/sunset data is unavailable: fixed 07:00-19:00 local.
function fallbackCycle(nowMs: number, isDayHint: boolean): DayCycle {
  const d = new Date(nowMs);
  const hours = d.getHours() + d.getMinutes() / 60;
  const t = clamp01((hours - 7) / 12);
  const day = isDayHint;
  const sunProgress = day ? (t > 0 && t < 1 ? t : 0.5) : 0;
  const nightHours = hours >= 19 ? hours - 19 : hours + 5;
  const moonProgress = day ? 0 : clamp01(nightHours / 12);
  return buildCycle(day, sunProgress, moonProgress, FALLBACK_HALF_DAY_MS);
}

// durationMs is the length of the CURRENT phase (day length when isDay,
// night length when !isDay): it anchors the golden/twilight window to a
// fixed real-time budget instead of a fraction of that length (see
// GOLDEN_WINDOW_MS above).
function buildCycle(isDay: boolean, sunProgress: number, moonProgress: number, durationMs: number): DayCycle {
  let golden: number;
  let night: number;
  if (isDay) {
    const edgeMs = Math.min(sunProgress, 1 - sunProgress) * durationMs;
    golden = 1 - smoothstep(0, GOLDEN_WINDOW_MS, edgeMs);
    night = 0;
  } else {
    const edgeMs = Math.min(moonProgress, 1 - moonProgress) * durationMs;
    const twilight = 1 - smoothstep(0, TWILIGHT_WINDOW_MS, edgeMs);
    golden = twilight * 0.55;
    night = 1 - twilight * 0.45;
  }
  const sun = arcPosition(sunProgress);
  const moon = arcPosition(moonProgress);
  return {
    isDay,
    sunProgress,
    moonProgress,
    golden,
    night,
    sunX: sun.x,
    sunY: sun.y,
    sunIntensity: isDay ? 1 : 0,
    moonX: moon.x,
    moonY: moon.y,
    moonIntensity: isDay ? 0 : 1,
  };
}

export function computeDayCycle(
  nowMs: number,
  times: SunTimes | null,
  isDayHint: boolean,
): DayCycle {
  if (!times || times.sunrises.length === 0 || times.sunsets.length === 0) {
    return fallbackCycle(nowMs, isDayHint);
  }

  // Daytime: some sunrise[i] <= now < the sunset that follows it.
  for (const rise of times.sunrises) {
    const set = times.sunsets.find((s) => s > rise);
    if (set !== undefined && set - rise >= MIN_EVENT_GAP_MS && nowMs >= rise && nowMs < set) {
      return buildCycle(true, (nowMs - rise) / (set - rise), 0, set - rise);
    }
  }

  // Nighttime: latest sunset <= now, earliest sunrise > now.
  const pastSets = times.sunsets.filter((s) => s <= nowMs);
  const nextRises = times.sunrises.filter((r) => r > nowMs);
  if (pastSets.length > 0 && nextRises.length > 0) {
    const set = Math.max(...pastSets);
    const rise = Math.min(...nextRises);
    if (rise - set >= MIN_EVENT_GAP_MS) {
      return buildCycle(false, 0, (nowMs - set) / (rise - set), rise - set);
    }
  }

  return fallbackCycle(nowMs, isDayHint);
}
