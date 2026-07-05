// Scene description - the visual vocabulary the renderer consumes.
// Pure data, no rendering concepts beyond normalized parameters.

import type { Precipitation } from "./weather";

// Broad visual family used by the palette mixer to tint the scene.
export type WeatherMood = "clear" | "cloudy" | "rain" | "snow" | "fog" | "storm";

export interface SceneParams {
  cloudCover: number;      // 0..1 how much of the sky is populated
  cloudDensity: number;    // 0..1 how thick the puffs read
  cloudSpeed: number;      // drift speed multiplier
  precipitation: Precipitation;
  precipIntensity: number; // 0..1
  thunderRate: number;     // average flashes per second, 0 when no storm
  fog: number;             // 0..1 fog wash opacity
  mood: WeatherMood;
  label: string;           // English label (fallback)
  labelKey: string;        // stable i18n key
}

export type RGB = [number, number, number];

// Final colors the shader consumes, already blended for time of day + mood.
export interface ScenePalette {
  cloudColor: RGB;   // lit side of the puffs
  cloudShadow: RGB;  // dense cores
  sunlight: RGB;     // rim light color
  fogColor: RGB;     // fog wash
  glowColor: RGB;    // horizon wash at golden hour
}
