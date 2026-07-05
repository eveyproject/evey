// HTTP adapter implementing the WeatherProvider port via Open-Meteo APIs.
// https://open-meteo.com/en/docs
//
// v2 additions over v1:
//   - daily sunrise/sunset (yesterday..tomorrow) with timeformat=unixtime,
//     so the day cycle uses real solar events instead of an hour heuristic.
//     Unix timestamps avoid any timezone parsing ambiguity: the browser
//     timezone never leaks into the city's solar times.
//   - request timeout via AbortController (a hung fetch left v1 on the
//     loading screen forever).

import type { GeoCoord, SunTimes, Units } from "../../domain/model/weather";
import type { CurrentWeatherResult, WeatherProvider } from "../../domain/ports/WeatherProvider";
import type { Precipitation } from "../../domain/model/weather";

const GEOCODE_URL = "https://geocoding-api.open-meteo.com/v1/search";
const FORECAST_URL = "https://api.open-meteo.com/v1/forecast";
const FETCH_TIMEOUT_MS = 10_000;

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
  daily?: {
    sunrise?: number[]; // unix seconds (timeformat=unixtime)
    sunset?: number[];
  };
}

function precipTypeFromCode(code: number): Precipitation {
  if ((code >= 51 && code <= 67) || (code >= 80 && code <= 82) || code >= 95) return "rain";
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return "snow";
  return "none";
}

function precipIntensityFromCode(code: number): number {
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

async function fetchJson<T>(url: string): Promise<T> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), FETCH_TIMEOUT_MS);
  try {
    const r = await fetch(url, { signal: ctrl.signal });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return (await r.json()) as T;
  } finally {
    clearTimeout(timer);
  }
}

function parseSunTimes(daily: ForecastResult["daily"]): SunTimes | null {
  const rises = daily?.sunrise;
  const sets = daily?.sunset;
  if (!rises || !sets || rises.length === 0 || sets.length === 0) return null;
  const toMs = (arr: number[]) => arr.filter((v) => Number.isFinite(v)).map((v) => v * 1000);
  const sunrises = toMs(rises);
  const sunsets = toMs(sets);
  if (sunrises.length === 0 || sunsets.length === 0) return null;
  return { sunrises, sunsets };
}

export class OpenMeteoProvider implements WeatherProvider {
  async geocode(city: string, lang = "en"): Promise<GeoCoord> {
    const safeLang = lang.slice(0, 2).toLowerCase();
    const url = `${GEOCODE_URL}?count=1&language=${safeLang}&name=${encodeURIComponent(city)}`;
    const data = await fetchJson<GeocodeResult>(url);
    if (!data.results || data.results.length === 0) throw new Error(`city not found: ${city}`);
    const r0 = data.results[0];
    return { lat: r0.latitude, lon: r0.longitude, name: r0.name };
  }

  async getCurrent(coord: GeoCoord, units: Units): Promise<CurrentWeatherResult> {
    const unitParam = units === "fahrenheit" ? "&temperature_unit=fahrenheit" : "";
    const url =
      `${FORECAST_URL}?latitude=${coord.lat}&longitude=${coord.lon}` +
      `&current_weather=true&windspeed_unit=kmh${unitParam}` +
      `&daily=sunrise,sunset&past_days=1&forecast_days=2` +
      `&timezone=auto&timeformat=unixtime`;
    const data = await fetchJson<ForecastResult>(url);
    if (!data.current_weather) throw new Error("no current_weather in forecast response");

    const cw = data.current_weather;
    const code = cw.weathercode;

    return {
      current: {
        conditionCode: code,
        precipitationType: precipTypeFromCode(code),
        precipIntensity: precipIntensityFromCode(code),
        hasThunder: code >= 95,
        temperatureDisplay: cw.temperature,
        windKph: cw.windspeed,
        isDay: cw.is_day === 1,
      },
      sunTimes: parseSunTimes(data.daily),
    };
  }
}
