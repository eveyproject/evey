// Blends the final shader colors from two independent axes:
//   1. time of day (day -> golden hour -> night), driven by the DayCycle
//   2. weather mood (clear / cloudy / rain / snow / fog / storm)
// Pure function: called on every state change, cheap enough to call per tick.

import type { DayCycle } from "./dayCycle";
import type { RGB, ScenePalette, WeatherMood } from "../model/scene";

// Time-of-day bases (from the v1 look, kept: it read well).
const DAY = {
  cloudColor:  [0.84, 0.87, 0.93] as RGB,
  cloudShadow: [0.18, 0.24, 0.36] as RGB,
  sunlight:    [1.00, 0.95, 0.85] as RGB,
  fogColor:    [0.90, 0.90, 0.90] as RGB,
  glowColor:   [1.00, 0.78, 0.55] as RGB,
};
const GOLDEN = {
  cloudColor:  [0.92, 0.78, 0.68] as RGB,
  cloudShadow: [0.30, 0.15, 0.30] as RGB,
  sunlight:    [1.00, 0.65, 0.45] as RGB,
  fogColor:    [0.85, 0.75, 0.68] as RGB,
  glowColor:   [1.00, 0.55, 0.25] as RGB,
};
const NIGHT = {
  cloudColor:  [0.55, 0.60, 0.72] as RGB,
  cloudShadow: [0.10, 0.14, 0.22] as RGB,
  sunlight:    [0.75, 0.80, 0.95] as RGB,
  fogColor:    [0.25, 0.28, 0.38] as RGB,
  glowColor:   [0.00, 0.00, 0.00] as RGB,
};

// Mood modifiers: multiplier on the time-of-day base plus a gray pull.
// grayPull desaturates toward its own luminance (overcast look).
interface MoodMod { mul: number; grayPull: number }
const MOOD_MODS: Record<WeatherMood, MoodMod> = {
  clear:  { mul: 1.00, grayPull: 0.00 },
  cloudy: { mul: 0.88, grayPull: 0.35 },
  rain:   { mul: 0.68, grayPull: 0.45 },
  snow:   { mul: 1.05, grayPull: 0.30 },
  fog:    { mul: 0.95, grayPull: 0.65 },
  storm:  { mul: 0.45, grayPull: 0.50 },
};

function lerp3(a: RGB, b: RGB, t: number): RGB {
  return [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
  ];
}

function applyMood(c: RGB, mod: MoodMod): RGB {
  const lum = c[0] * 0.299 + c[1] * 0.587 + c[2] * 0.114;
  return [
    (c[0] + (lum - c[0]) * mod.grayPull) * mod.mul,
    (c[1] + (lum - c[1]) * mod.grayPull) * mod.mul,
    (c[2] + (lum - c[2]) * mod.grayPull) * mod.mul,
  ];
}

export function mixPalette(mood: WeatherMood, cycle: DayCycle): ScenePalette {
  const mod = MOOD_MODS[mood];
  const g = cycle.golden;
  const n = cycle.night;
  const blend = (key: keyof typeof DAY): RGB =>
    applyMood(lerp3(lerp3(DAY[key], GOLDEN[key], g), NIGHT[key], n), mod);
  return {
    cloudColor: blend("cloudColor"),
    cloudShadow: blend("cloudShadow"),
    sunlight: blend("sunlight"),
    fogColor: blend("fogColor"),
    // Glow keeps full mood color but vanishes with night.
    glowColor: lerp3(lerp3(DAY.glowColor, GOLDEN.glowColor, g), NIGHT.glowColor, n),
  };
}
