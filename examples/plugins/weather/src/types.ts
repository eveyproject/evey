// Shared types for the weather plugin front module.

export interface WeatherState {
  city: string;
  temp: number;
  code: number;
  isDay: boolean;
  units: "celsius" | "fahrenheit";
}

export interface WeatherProfile {
  // Sky gradient: top → bottom RGB triplets in 0..1.
  skyTop: [number, number, number];
  skyBottom: [number, number, number];
  // Orb base color and accent (used by the shader).
  orbColor: [number, number, number];
  // Effect flags driven by the weather code.
  hasRain: boolean;
  hasSnow: boolean;
  hasStorm: boolean;
  hasFog: boolean;
  // Cloud coverage 0..1.
  cloudCoverage: number;
  // Sun/moon brightness 0..1.
  sunBrightness: number;
  label: string;
}

// Open-Meteo WMO weather codes → visual profile.
// https://open-meteo.com/en/docs (codes 0..99 mapped to bands)
export function profileForWeather(code: number, isDay: boolean): WeatherProfile {
  const dayClear: Pick<WeatherProfile, "skyTop" | "skyBottom"> = {
    skyTop: [0.20, 0.45, 0.85],
    skyBottom: [0.95, 0.78, 0.55],
  };
  const nightClear: Pick<WeatherProfile, "skyTop" | "skyBottom"> = {
    skyTop: [0.04, 0.06, 0.18],
    skyBottom: [0.20, 0.18, 0.40],
  };
  const base = isDay ? dayClear : nightClear;

  if (code === 0) {
    return {
      ...base,
      orbColor: isDay ? [1.0, 0.85, 0.45] : [0.85, 0.85, 0.95],
      hasRain: false,
      hasSnow: false,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.0,
      sunBrightness: 1.0,
      label: isDay ? "Clear" : "Clear night",
    };
  }
  if (code === 1 || code === 2) {
    return {
      ...base,
      orbColor: isDay ? [0.95, 0.80, 0.55] : [0.75, 0.78, 0.92],
      hasRain: false,
      hasSnow: false,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.45,
      sunBrightness: 0.7,
      label: "Partly cloudy",
    };
  }
  if (code === 3) {
    return {
      skyTop: [0.45, 0.50, 0.58],
      skyBottom: [0.72, 0.74, 0.78],
      orbColor: [0.65, 0.68, 0.74],
      hasRain: false,
      hasSnow: false,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.85,
      sunBrightness: 0.15,
      label: "Overcast",
    };
  }
  if (code >= 45 && code <= 48) {
    return {
      skyTop: [0.62, 0.64, 0.66],
      skyBottom: [0.85, 0.86, 0.86],
      orbColor: [0.80, 0.82, 0.85],
      hasRain: false,
      hasSnow: false,
      hasStorm: false,
      hasFog: true,
      cloudCoverage: 0.95,
      sunBrightness: 0.1,
      label: "Fog",
    };
  }
  if (code >= 51 && code <= 67) {
    return {
      skyTop: [0.30, 0.35, 0.45],
      skyBottom: [0.55, 0.60, 0.66],
      orbColor: [0.55, 0.65, 0.85],
      hasRain: true,
      hasSnow: false,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.9,
      sunBrightness: 0.0,
      label: "Rain",
    };
  }
  if (code >= 71 && code <= 77) {
    return {
      skyTop: [0.62, 0.66, 0.74],
      skyBottom: [0.86, 0.90, 0.95],
      orbColor: [0.95, 0.96, 1.0],
      hasRain: false,
      hasSnow: true,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.85,
      sunBrightness: 0.0,
      label: "Snow",
    };
  }
  if (code >= 80 && code <= 82) {
    return {
      skyTop: [0.34, 0.40, 0.50],
      skyBottom: [0.58, 0.63, 0.68],
      orbColor: [0.55, 0.65, 0.85],
      hasRain: true,
      hasSnow: false,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.95,
      sunBrightness: 0.0,
      label: "Showers",
    };
  }
  if (code >= 85 && code <= 86) {
    return {
      skyTop: [0.65, 0.70, 0.78],
      skyBottom: [0.90, 0.92, 0.96],
      orbColor: [0.95, 0.96, 1.0],
      hasRain: false,
      hasSnow: true,
      hasStorm: false,
      hasFog: false,
      cloudCoverage: 0.85,
      sunBrightness: 0.0,
      label: "Snow showers",
    };
  }
  if (code >= 95) {
    return {
      skyTop: [0.10, 0.10, 0.18],
      skyBottom: [0.25, 0.25, 0.35],
      orbColor: [0.45, 0.45, 0.65],
      hasRain: true,
      hasSnow: false,
      hasStorm: true,
      hasFog: false,
      cloudCoverage: 1.0,
      sunBrightness: 0.0,
      label: "Thunderstorm",
    };
  }
  return {
    ...base,
    orbColor: [0.7, 0.75, 0.85],
    hasRain: false,
    hasSnow: false,
    hasStorm: false,
    hasFog: false,
    cloudCoverage: 0.5,
    sunBrightness: 0.3,
    label: "Unknown",
  };
}
