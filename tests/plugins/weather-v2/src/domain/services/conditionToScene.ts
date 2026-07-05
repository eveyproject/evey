// WMO weather code -> SceneParams.
// Codes: https://open-meteo.com/en/docs (weathercode field)
// Pure function, table-driven: one entry per WMO band instead of v1's
// 300-line if-cascade. Time-of-day coloring is NOT decided here; the
// palette mixer blends mood + day cycle at render time.

import type { WeatherSnapshot } from "../model/weather";
import type { SceneParams, WeatherMood } from "../model/scene";
import type { Precipitation } from "../model/weather";

interface SceneRow {
  cover: number;
  density: number;
  speed: number;
  precip: Precipitation;
  intensity: number;
  thunder: number;
  fog: number;
  mood: WeatherMood;
  label: string;
  labelKey: string;
}

function row(
  cover: number, density: number, speed: number,
  precip: Precipitation, intensity: number, thunder: number, fog: number,
  mood: WeatherMood, label: string, labelKey: string,
): SceneRow {
  return { cover, density, speed, precip, intensity, thunder, fog, mood, label, labelKey };
}

// Keyed by exact WMO code.
const TABLE: Record<number, SceneRow> = {
  0:  row(0.00, 0.00, 0.10, "none", 0.00, 0, 0.00, "clear",  "Clear",            "clear_day"),
  1:  row(0.15, 0.20, 0.15, "none", 0.00, 0, 0.00, "clear",  "Mainly clear",     "mainly_clear"),
  2:  row(0.45, 0.40, 0.20, "none", 0.00, 0, 0.00, "clear",  "Partly cloudy",    "partly_cloudy"),
  3:  row(0.90, 0.75, 0.18, "none", 0.00, 0, 0.05, "cloudy", "Overcast",         "overcast"),
  45: row(0.95, 0.85, 0.05, "none", 0.00, 0, 0.88, "fog",    "Fog",              "fog"),
  48: row(0.95, 0.85, 0.05, "none", 0.00, 0, 0.88, "fog",    "Rime fog",         "rime_fog"),
  51: row(0.85, 0.65, 0.22, "rain", 0.20, 0, 0.15, "rain",   "Light drizzle",    "light_drizzle"),
  53: row(0.85, 0.65, 0.22, "rain", 0.45, 0, 0.15, "rain",   "Drizzle",          "drizzle"),
  55: row(0.85, 0.65, 0.22, "rain", 0.65, 0, 0.15, "rain",   "Dense drizzle",    "dense_drizzle"),
  56: row(0.88, 0.70, 0.20, "rain", 0.25, 0, 0.20, "snow",   "Light freezing drizzle", "light_freezing_drizzle"),
  57: row(0.88, 0.70, 0.20, "rain", 0.55, 0, 0.20, "snow",   "Freezing drizzle", "freezing_drizzle"),
  61: row(0.92, 0.80, 0.30, "rain", 0.30, 0, 0.10, "rain",   "Light rain",       "light_rain"),
  63: row(0.92, 0.80, 0.30, "rain", 0.60, 0, 0.10, "rain",   "Moderate rain",    "rain"),
  65: row(0.92, 0.80, 0.30, "rain", 0.90, 0, 0.10, "rain",   "Heavy rain",       "heavy_rain"),
  66: row(0.90, 0.78, 0.28, "rain", 0.40, 0, 0.15, "rain",   "Light freezing rain", "light_freezing_rain"),
  67: row(0.90, 0.78, 0.28, "rain", 0.75, 0, 0.15, "rain",   "Freezing rain",    "freezing_rain"),
  71: row(0.88, 0.72, 0.15, "snow", 0.25, 0, 0.12, "snow",   "Light snow",       "slight_snow"),
  73: row(0.88, 0.72, 0.15, "snow", 0.55, 0, 0.12, "snow",   "Moderate snow",    "snow"),
  75: row(0.88, 0.72, 0.15, "snow", 0.85, 0, 0.12, "snow",   "Heavy snow",       "heavy_snow"),
  77: row(0.85, 0.68, 0.12, "snow", 0.30, 0, 0.10, "snow",   "Snow grains",      "snow_grains"),
  80: row(0.80, 0.72, 0.35, "rain", 0.40, 0, 0.05, "rain",   "Light showers",    "light_showers"),
  81: row(0.80, 0.72, 0.35, "rain", 0.70, 0, 0.05, "rain",   "Showers",          "showers"),
  82: row(0.80, 0.72, 0.35, "rain", 0.95, 0, 0.05, "rain",   "Heavy showers",    "violent_showers"),
  85: row(0.82, 0.68, 0.20, "snow", 0.35, 0, 0.08, "snow",   "Light snow showers", "light_snow_showers"),
  86: row(0.82, 0.68, 0.20, "snow", 0.72, 0, 0.08, "snow",   "Heavy snow showers", "heavy_snow_showers"),
  95: row(1.00, 0.92, 0.50, "rain", 0.70, 0.4, 0.05, "storm", "Thunderstorm",    "thunderstorm"),
  96: row(1.00, 0.95, 0.60, "rain", 0.90, 0.8, 0.05, "storm", "Thunderstorm with hail", "thunderstorm_hail"),
  99: row(1.00, 0.95, 0.60, "rain", 0.90, 0.8, 0.05, "storm", "Heavy thunderstorm with hail", "violent_thunderstorm_hail"),
};

const UNKNOWN = row(0.50, 0.50, 0.20, "none", 0, 0, 0, "clear", "Unknown", "unknown");

export function conditionToScene(snap: WeatherSnapshot): SceneParams {
  const r = TABLE[snap.conditionCode] ?? UNKNOWN;
  // Clear-sky label depends on day/night.
  const isClearSky = snap.conditionCode === 0;
  return {
    cloudCover: r.cover,
    cloudDensity: r.density,
    cloudSpeed: r.speed,
    precipitation: r.precip,
    precipIntensity: r.intensity,
    thunderRate: r.thunder,
    fog: r.fog,
    mood: r.mood,
    label: isClearSky && !snap.isDay ? "Clear night" : r.label,
    labelKey: isClearSky && !snap.isDay ? "clear_night" : r.labelKey,
  };
}
