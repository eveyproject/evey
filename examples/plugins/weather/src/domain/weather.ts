// Pure domain types — zero React, zero Three. Testable in isolation.

export interface GeoCoord {
  lat: number;
  lon: number;
  name: string;
}

export interface WeatherSnapshot {
  conditionCode: number;     // WMO code 0..99
  cloudCoverPct: number;     // 0..100 (from WMO band heuristic)
  precipitationType: "none" | "rain" | "snow";
  precipIntensity: number;   // 0..1
  hasThunder: boolean;
  temperatureC: number;
  temperatureDisplay: number; // celsius or fahrenheit depending on units
  windKph: number;
  isDay: boolean;
  units: "celsius" | "fahrenheit";
  cityName: string;
}

export interface SceneParams {
  cloudCover: number;        // 0..1
  cloudDensity: number;      // 0..1
  cloudSpeed: number;        // arbitrary units for shader animation
  precipitation: "none" | "rain" | "snow";
  precipIntensity: number;   // 0..1
  windVec: [number, number]; // normalized direction
  thunderRate: number;       // flashes/sec, 0 if no storm
  fog: number;               // 0..1
  isDay: boolean;
  sunAltitude: number;       // -π/2..π/2
  sunAzimuth: number;        // -π..π
  moonPhase: number;         // 0..1
  palette: {
    zenith: [number, number, number];
    horizon: [number, number, number];
    sun: [number, number, number];
    fog: [number, number, number];
  };
  label: string;
  labelKey: string;  // stable i18n key, e.g. "partly_cloudy"
}
