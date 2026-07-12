// HTTP adapter implementing the WeatherProvider port via Open-Meteo APIs.
// https://open-meteo.com/en/docs

import type { GeoCoord, WeatherSnapshot } from "../domain/weather";
import type { WeatherProvider } from "../domain/ports/WeatherProvider";

const GEOCODE_URL = "https://geocoding-api.open-meteo.com/v1/search";
const FORECAST_URL = "https://api.open-meteo.com/v1/forecast";

interface GeocodeResult {
  results?: Array<{ latitude: number; longitude: number; name: string }>;
}

interface ForecastResult {
  current_weather?: {
    temperature: number;
    weathercode: number;
    is_day: 0 | 1;
    windspeed: number;
  };
  current?: {
    cloud_cover?: number;
  };
}

// Derive cloudCoverPct from WMO code (rough heuristic - code is authoritative,
// but we want a continuous value for shader interpolation).
function cloudCoverFromCode(code: number): number {
  if (code === 0) return 0;
  if (code === 1) return 15;
  if (code === 2) return 45;
  if (code === 3) return 90;
  if (code >= 45 && code <= 48) return 95;
  if (code >= 51 && code <= 57) return 85;
  if (code >= 61 && code <= 67) return 92;
  if (code >= 71 && code <= 77) return 88;
  if (code >= 80 && code <= 82) return 80;
  if (code >= 85 && code <= 86) return 82;
  if (code >= 95) return 100;
  return 50;
}

function precipTypeFromCode(code: number): "none" | "rain" | "snow" {
  if ((code >= 51 && code <= 67) || (code >= 80 && code <= 82) || code >= 95) return "rain";
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return "snow";
  return "none";
}

function precipIntensityFromCode(code: number): number {
  // Light: 0.2-0.4, moderate: 0.5-0.7, heavy/storm: 0.8-1.0
  if (code === 51 || code === 61 || code === 71 || code === 80 || code === 85) return 0.25;
  if (code === 53 || code === 63 || code === 73 || code === 81) return 0.55;
  if (code === 55 || code === 65 || code === 75 || code === 82 || code === 86) return 0.85;
  if (code === 56 || code === 66) return 0.30;
  if (code === 57 || code === 67) return 0.65;
  if (code === 77) return 0.30;
  if (code === 95) return 0.70;
  if (code === 96 || code === 99) return 0.90;
  return 0;
}

export class OpenMeteoWeatherProvider implements WeatherProvider {
  async geocode(city: string, lang = "en"): Promise<GeoCoord> {
    const safeLang = lang.slice(0, 2).toLowerCase();
    const url = `${GEOCODE_URL}?count=1&language=${safeLang}&name=${encodeURIComponent(city)}`;
    const r = await fetch(url);
    if (!r.ok) throw new Error(`geocoding HTTP ${r.status}`);
    const data: GeocodeResult = await r.json();
    if (!data.results || data.results.length === 0) throw new Error(`city not found: ${city}`);
    const r0 = data.results[0];
    return { lat: r0.latitude, lon: r0.longitude, name: r0.name };
  }

  async getCurrent(
    coord: GeoCoord,
    units: "celsius" | "fahrenheit"
  ): Promise<Omit<WeatherSnapshot, "cityName" | "units">> {
    const unitParam = units === "fahrenheit" ? "&temperature_unit=fahrenheit" : "";
    const url = `${FORECAST_URL}?current_weather=true&latitude=${coord.lat}&longitude=${coord.lon}${unitParam}&windspeed_unit=kmh`;
    const r = await fetch(url);
    if (!r.ok) throw new Error(`forecast HTTP ${r.status}`);
    const data: ForecastResult = await r.json();
    if (!data.current_weather) throw new Error("no current_weather in forecast response");

    const cw = data.current_weather;
    const code = cw.weathercode;

    return {
      conditionCode: code,
      cloudCoverPct: cloudCoverFromCode(code),
      precipitationType: precipTypeFromCode(code),
      precipIntensity: precipIntensityFromCode(code),
      hasThunder: code >= 95,
      temperatureC: units === "fahrenheit" ? (cw.temperature - 32) * (5 / 9) : cw.temperature,
      temperatureDisplay: cw.temperature,
      windKph: cw.windspeed,
      isDay: cw.is_day === 1,
    };
  }
}
