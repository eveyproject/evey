// Domain model - pure TypeScript value objects. Zero runtime dependencies,
// zero DOM, zero WebGL. Everything here is testable in isolation.

export type Units = "celsius" | "fahrenheit";
export type Precipitation = "none" | "rain" | "snow";

export interface GeoCoord {
  lat: number;
  lon: number;
  name: string;
}

// One geocoding match, carrying enough to DISAMBIGUATE homonyms (e.g. Brest,
// France vs Brest, Belarus): the region and country are shown in the settings
// picker so the user resolves the ambiguity once, and the coordinates are then
// stored - the widget never re-geocodes a bare name and lands on the wrong city.
export interface GeoCandidate {
  lat: number;
  lon: number;
  name: string;
  admin1?: string; // region / state / département
  country?: string;
  countryCode?: string;
}

// Sunrise/sunset events as epoch milliseconds, spanning yesterday..tomorrow
// so the day/night cycle can always find the bracketing events regardless of
// the local hour. Provided by the weather adapter (Open-Meteo daily API);
// null when the API omits them (polar day/night or partial response).
export interface SunTimes {
  sunrises: number[];
  sunsets: number[];
}

export interface WeatherSnapshot {
  conditionCode: number;      // WMO code 0..99
  precipitationType: Precipitation;
  precipIntensity: number;    // 0..1
  hasThunder: boolean;
  temperatureDisplay: number; // in the requested units
  windKph: number;
  isDay: boolean;             // authoritative flag from the provider
  units: Units;
  cityName: string;
}
