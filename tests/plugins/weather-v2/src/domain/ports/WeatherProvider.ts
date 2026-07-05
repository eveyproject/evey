// Driven port: how the domain obtains weather data.

import type { GeoCoord, SunTimes, Units, WeatherSnapshot } from "../model/weather";

export interface CurrentWeatherResult {
  current: Omit<WeatherSnapshot, "cityName" | "units">;
  sunTimes: SunTimes | null;
}

export interface WeatherProvider {
  geocode(city: string, lang: string): Promise<GeoCoord>;
  getCurrent(coord: GeoCoord, units: Units): Promise<CurrentWeatherResult>;
}
