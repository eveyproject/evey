// WMO weather code → SceneParams.
// Codes: https://open-meteo.com/en/docs (weathercode field)
// Pure function - no side-effects, no imports from React/Three.

import type { WeatherSnapshot, SceneParams } from "../weather";
import { sunPosition, moonPhase } from "./sunPosition";

// Palette presets
const DAY_CLEAR = {
  zenith:  [0.18, 0.42, 0.82] as [number, number, number],
  horizon: [0.85, 0.72, 0.52] as [number, number, number],
  sun:     [1.00, 0.92, 0.65] as [number, number, number],
  fog:     [0.95, 0.92, 0.88] as [number, number, number],
};
const NIGHT_CLEAR = {
  zenith:  [0.03, 0.05, 0.15] as [number, number, number],
  horizon: [0.12, 0.15, 0.30] as [number, number, number],
  sun:     [0.78, 0.82, 0.95] as [number, number, number], // moon color
  fog:     [0.10, 0.12, 0.20] as [number, number, number],
};
const CLOUDY = {
  zenith:  [0.38, 0.42, 0.50] as [number, number, number],
  horizon: [0.65, 0.67, 0.72] as [number, number, number],
  sun:     [0.80, 0.80, 0.80] as [number, number, number],
  fog:     [0.72, 0.74, 0.76] as [number, number, number],
};
const RAINY = {
  zenith:  [0.22, 0.26, 0.36] as [number, number, number],
  horizon: [0.42, 0.46, 0.55] as [number, number, number],
  sun:     [0.60, 0.65, 0.72] as [number, number, number],
  fog:     [0.50, 0.54, 0.60] as [number, number, number],
};
const SNOWY = {
  zenith:  [0.55, 0.60, 0.72] as [number, number, number],
  horizon: [0.82, 0.86, 0.92] as [number, number, number],
  sun:     [0.95, 0.96, 1.00] as [number, number, number],
  fog:     [0.88, 0.90, 0.94] as [number, number, number],
};
const FOGGY = {
  zenith:  [0.58, 0.60, 0.62] as [number, number, number],
  horizon: [0.82, 0.84, 0.84] as [number, number, number],
  sun:     [0.90, 0.90, 0.88] as [number, number, number],
  fog:     [0.85, 0.86, 0.86] as [number, number, number],
};
const STORM = {
  zenith:  [0.08, 0.08, 0.14] as [number, number, number],
  horizon: [0.20, 0.20, 0.30] as [number, number, number],
  sun:     [0.40, 0.40, 0.58] as [number, number, number],
  fog:     [0.25, 0.25, 0.35] as [number, number, number],
};

function base(snap: WeatherSnapshot, now: Date): Pick<SceneParams, "isDay" | "sunAltitude" | "sunAzimuth" | "moonPhase" | "windVec"> {
  const sp = sunPosition(now);
  const mp = moonPhase(now);
  const windAngle = Math.random() * Math.PI * 2;
  return {
    isDay: snap.isDay,
    sunAltitude: sp.altitude,
    sunAzimuth: sp.azimuth,
    moonPhase: mp,
    windVec: [Math.cos(windAngle), Math.sin(windAngle)],
  };
}

export function conditionToScene(snap: WeatherSnapshot, now: Date): SceneParams {
  const b = base(snap, now);
  const code = snap.conditionCode;

  // 0 - Clear sky
  if (code === 0) {
    return {
      ...b,
      cloudCover: 0.0,
      cloudDensity: 0.0,
      cloudSpeed: 0.1,
      precipitation: "none",
      precipIntensity: 0.0,
      thunderRate: 0,
      fog: 0.0,
      palette: snap.isDay ? DAY_CLEAR : NIGHT_CLEAR,
      label: snap.isDay ? "Clear" : "Clear night",
      labelKey: snap.isDay ? "clear_day" : "clear_night",
    };
  }

  // 1 - Mainly clear
  if (code === 1) {
    return {
      ...b,
      cloudCover: 0.15,
      cloudDensity: 0.2,
      cloudSpeed: 0.15,
      precipitation: "none",
      precipIntensity: 0.0,
      thunderRate: 0,
      fog: 0.0,
      palette: snap.isDay ? DAY_CLEAR : NIGHT_CLEAR,
      label: "Mainly clear",
      labelKey: "mainly_clear",
    };
  }

  // 2 - Partly cloudy
  if (code === 2) {
    return {
      ...b,
      cloudCover: 0.45,
      cloudDensity: 0.4,
      cloudSpeed: 0.2,
      precipitation: "none",
      precipIntensity: 0.0,
      thunderRate: 0,
      fog: 0.0,
      palette: snap.isDay ? DAY_CLEAR : NIGHT_CLEAR,
      label: "Partly cloudy",
      labelKey: "partly_cloudy",
    };
  }

  // 3 - Overcast
  if (code === 3) {
    return {
      ...b,
      cloudCover: 0.90,
      cloudDensity: 0.75,
      cloudSpeed: 0.18,
      precipitation: "none",
      precipIntensity: 0.0,
      thunderRate: 0,
      fog: 0.05,
      palette: CLOUDY,
      label: "Overcast",
      labelKey: "overcast",
    };
  }

  // 45, 48 - Fog / depositing rime fog
  if (code === 45 || code === 48) {
    return {
      ...b,
      cloudCover: 0.95,
      cloudDensity: 0.85,
      cloudSpeed: 0.05,
      precipitation: "none",
      precipIntensity: 0.0,
      thunderRate: 0,
      fog: 0.88,
      palette: FOGGY,
      label: code === 48 ? "Rime fog" : "Fog",
      labelKey: code === 48 ? "rime_fog" : "fog",
    };
  }

  // 51, 53, 55 - Drizzle (light/moderate/dense)
  if (code >= 51 && code <= 55) {
    const intensity = code === 51 ? 0.2 : code === 53 ? 0.45 : 0.65;
    return {
      ...b,
      cloudCover: 0.85,
      cloudDensity: 0.65,
      cloudSpeed: 0.22,
      precipitation: "rain",
      precipIntensity: intensity,
      thunderRate: 0,
      fog: 0.15,
      palette: RAINY,
      label: code === 51 ? "Light drizzle" : code === 53 ? "Drizzle" : "Dense drizzle",
      labelKey: code === 51 ? "light_drizzle" : code === 53 ? "drizzle" : "dense_drizzle",
    };
  }

  // 56, 57 - Freezing drizzle
  if (code === 56 || code === 57) {
    return {
      ...b,
      cloudCover: 0.88,
      cloudDensity: 0.70,
      cloudSpeed: 0.20,
      precipitation: "rain",
      precipIntensity: code === 56 ? 0.25 : 0.55,
      thunderRate: 0,
      fog: 0.20,
      palette: SNOWY,
      label: code === 56 ? "Light freezing drizzle" : "Freezing drizzle",
      labelKey: code === 56 ? "light_freezing_drizzle" : "freezing_drizzle",
    };
  }

  // 61, 63, 65 - Rain (slight/moderate/heavy)
  if (code >= 61 && code <= 65) {
    const intensity = code === 61 ? 0.30 : code === 63 ? 0.60 : 0.90;
    return {
      ...b,
      cloudCover: 0.92,
      cloudDensity: 0.80,
      cloudSpeed: 0.30,
      precipitation: "rain",
      precipIntensity: intensity,
      thunderRate: 0,
      fog: 0.10,
      palette: RAINY,
      label: code === 61 ? "Light rain" : code === 63 ? "Moderate rain" : "Heavy rain",
      labelKey: code === 61 ? "light_rain" : code === 63 ? "rain" : "heavy_rain",
    };
  }

  // 66, 67 - Freezing rain
  if (code === 66 || code === 67) {
    return {
      ...b,
      cloudCover: 0.90,
      cloudDensity: 0.78,
      cloudSpeed: 0.28,
      precipitation: "rain",
      precipIntensity: code === 66 ? 0.40 : 0.75,
      thunderRate: 0,
      fog: 0.15,
      palette: RAINY,
      label: code === 66 ? "Light freezing rain" : "Freezing rain",
      labelKey: code === 66 ? "light_freezing_rain" : "freezing_rain",
    };
  }

  // 71, 73, 75 - Snow fall (slight/moderate/heavy)
  if (code >= 71 && code <= 75) {
    const intensity = code === 71 ? 0.25 : code === 73 ? 0.55 : 0.85;
    return {
      ...b,
      cloudCover: 0.88,
      cloudDensity: 0.72,
      cloudSpeed: 0.15,
      precipitation: "snow",
      precipIntensity: intensity,
      thunderRate: 0,
      fog: 0.12,
      palette: SNOWY,
      label: code === 71 ? "Light snow" : code === 73 ? "Moderate snow" : "Heavy snow",
      labelKey: code === 71 ? "slight_snow" : code === 73 ? "snow" : "heavy_snow",
    };
  }

  // 77 - Snow grains
  if (code === 77) {
    return {
      ...b,
      cloudCover: 0.85,
      cloudDensity: 0.68,
      cloudSpeed: 0.12,
      precipitation: "snow",
      precipIntensity: 0.30,
      thunderRate: 0,
      fog: 0.10,
      palette: SNOWY,
      label: "Snow grains",
      labelKey: "snow_grains",
    };
  }

  // 80, 81, 82 - Rain showers (slight/moderate/violent)
  if (code >= 80 && code <= 82) {
    const intensity = code === 80 ? 0.40 : code === 81 ? 0.70 : 0.95;
    return {
      ...b,
      cloudCover: 0.80,
      cloudDensity: 0.72,
      cloudSpeed: 0.35,
      precipitation: "rain",
      precipIntensity: intensity,
      thunderRate: 0,
      fog: 0.05,
      palette: RAINY,
      label: code === 80 ? "Light showers" : code === 81 ? "Showers" : "Heavy showers",
      labelKey: code === 80 ? "light_showers" : code === 81 ? "showers" : "violent_showers",
    };
  }

  // 85, 86 - Snow showers (slight/heavy)
  if (code === 85 || code === 86) {
    return {
      ...b,
      cloudCover: 0.82,
      cloudDensity: 0.68,
      cloudSpeed: 0.20,
      precipitation: "snow",
      precipIntensity: code === 85 ? 0.35 : 0.72,
      thunderRate: 0,
      fog: 0.08,
      palette: SNOWY,
      label: code === 85 ? "Light snow showers" : "Heavy snow showers",
      labelKey: code === 85 ? "light_snow_showers" : "heavy_snow_showers",
    };
  }

  // 95 - Thunderstorm (slight/moderate)
  if (code === 95) {
    return {
      ...b,
      cloudCover: 1.0,
      cloudDensity: 0.92,
      cloudSpeed: 0.50,
      precipitation: "rain",
      precipIntensity: 0.70,
      thunderRate: 0.4,
      fog: 0.05,
      palette: STORM,
      label: "Thunderstorm",
      labelKey: "thunderstorm",
    };
  }

  // 96, 99 - Thunderstorm with hail
  if (code === 96 || code === 99) {
    return {
      ...b,
      cloudCover: 1.0,
      cloudDensity: 0.95,
      cloudSpeed: 0.60,
      precipitation: "rain",
      precipIntensity: 0.90,
      thunderRate: 0.8,
      fog: 0.05,
      palette: STORM,
      label: code === 96 ? "Thunderstorm with hail" : "Heavy thunderstorm with hail",
      labelKey: code === 96 ? "thunderstorm_hail" : "violent_thunderstorm_hail",
    };
  }

  // Fallback - unknown code
  return {
    ...b,
    cloudCover: 0.50,
    cloudDensity: 0.50,
    cloudSpeed: 0.20,
    precipitation: "none",
    precipIntensity: 0.0,
    thunderRate: 0,
    fog: 0.0,
    palette: snap.isDay ? DAY_CLEAR : NIGHT_CLEAR,
    label: "Unknown",
    labelKey: "unknown",
  };
}
