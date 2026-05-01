// Port interface — the domain defines the shape, adapters implement it.
import type { GeoCoord, WeatherSnapshot } from "../weather";

export interface WeatherProvider {
  geocode(city: string, lang?: string): Promise<GeoCoord>;
  getCurrent(coord: GeoCoord, units: "celsius" | "fahrenheit"): Promise<Omit<WeatherSnapshot, "cityName" | "units">>;
}
