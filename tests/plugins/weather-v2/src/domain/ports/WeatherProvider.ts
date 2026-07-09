// Driven port: how the domain obtains weather data.

import type { GeoCandidate, GeoCoord, SunTimes, Units, WeatherSnapshot } from "../model/weather";

export interface CurrentWeatherResult {
  current: Omit<WeatherSnapshot, "cityName" | "units">;
  sunTimes: SunTimes | null;
}

export interface WeatherProvider {
  // Single best match (name only) - the legacy resolution kept for stored prefs
  // that predate the picker, or when the widget has only a city name.
  geocode(city: string, lang: string): Promise<GeoCoord>;
  // Ranked candidates for the disambiguation picker (region + country each).
  geocodeSearch(city: string, lang: string): Promise<GeoCandidate[]>;
  getCurrent(coord: GeoCoord, units: Units): Promise<CurrentWeatherResult>;
}
