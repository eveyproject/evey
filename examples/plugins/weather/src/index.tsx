// Entry point for the weather plugin's front module.
//
// Mounts a React app into document.body that renders:
//   - an R3F Canvas with a procedural sky + a noise-displaced orb that
//     adapts to the current weather, plus rain/snow particle systems
//     and a storm flash light when applicable
//   - an HTML overlay with the temperature + city + condition label,
//     anchored top-right and transparent over the canvas
//
// Bridge contract (plugin API): window.evey.{onInit, resize}.
// onInit fires once with { context, config, locale, theme }; we read
// config.city + config.units to drive the API requests.

import { createRoot } from "react-dom/client";
import { Canvas } from "@react-three/fiber";
import { useEffect, useState } from "react";
import { profileForWeather, type WeatherState } from "./types";
import { Scene } from "./scene";

declare global {
  interface Window {
    evey: {
      onInit(cb: (data: {
        context?: Record<string, unknown>;
        config?: { city?: string; units?: "celsius" | "fahrenheit" };
        locale?: string;
        theme?: string;
      }) => void): void;
      resize(height: number): void;
      navigate(path: string): void;
      toast(level: "success" | "error" | "info", message: string): void;
    };
  }
}

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
  };
}

async function geocode(city: string): Promise<{ lat: number; lon: number; name: string }> {
  const url = `${GEOCODE_URL}?count=1&language=en&name=${encodeURIComponent(city)}`;
  const r = await fetch(url);
  if (!r.ok) throw new Error(`geocoding HTTP ${r.status}`);
  const data: GeocodeResult = await r.json();
  if (!data.results || data.results.length === 0) throw new Error(`city not found: ${city}`);
  const r0 = data.results[0];
  return { lat: r0.latitude, lon: r0.longitude, name: r0.name };
}

async function fetchWeather(lat: number, lon: number, units: "celsius" | "fahrenheit"): Promise<{ temp: number; code: number; isDay: boolean }> {
  const unitParam = units === "fahrenheit" ? "&temperature_unit=fahrenheit" : "";
  const r = await fetch(`${FORECAST_URL}?current_weather=true&latitude=${lat}&longitude=${lon}${unitParam}`);
  if (!r.ok) throw new Error(`forecast HTTP ${r.status}`);
  const data: ForecastResult = await r.json();
  if (!data.current_weather) throw new Error("no current_weather");
  return {
    temp: data.current_weather.temperature,
    code: data.current_weather.weathercode,
    isDay: data.current_weather.is_day === 1,
  };
}

function App({ city, units }: { city: string; units: "celsius" | "fahrenheit" }) {
  const [state, setState] = useState<WeatherState | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const geo = await geocode(city);
        const w = await fetchWeather(geo.lat, geo.lon, units);
        if (cancelled) return;
        setState({ city: geo.name, temp: w.temp, code: w.code, isDay: w.isDay, units });
      } catch (e) {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : "fetch failed");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [city, units]);

  // Notify the host of the desired iframe height once after first paint.
  useEffect(() => {
    const h = 240;
    window.evey?.resize(h);
  }, []);

  const profile = state
    ? profileForWeather(state.code, state.isDay)
    : profileForWeather(0, true);

  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "240px",
        borderRadius: "16px",
        overflow: "hidden",
        background: "transparent",
        boxShadow: "0 12px 32px rgba(0,0,0,0.25)",
      }}
    >
      <Canvas
        camera={{ position: [0, 0, 4.5], fov: 35 }}
        dpr={[1, 2]}
        gl={{ antialias: true, alpha: true }}
        style={{ position: "absolute", inset: 0 }}
      >
        <Scene profile={profile} />
      </Canvas>
      <div
        style={{
          position: "absolute",
          top: 16,
          right: 20,
          color: "#ffffff",
          fontFamily: "system-ui, -apple-system, sans-serif",
          textAlign: "right",
          textShadow: "0 2px 6px rgba(0,0,0,0.55)",
          pointerEvents: "none",
        }}
      >
        <div style={{ fontSize: 44, fontWeight: 600, letterSpacing: "-0.02em", lineHeight: 1 }}>
          {state ? `${Math.round(state.temp)}${state.units === "fahrenheit" ? "°F" : "°C"}` : "—"}
        </div>
        <div style={{ fontSize: 13, opacity: 0.85, marginTop: 6, textTransform: "uppercase", letterSpacing: "0.08em" }}>
          {error
            ? "Unavailable"
            : state
              ? `${state.city} · ${profile.label}`
              : "Loading…"}
        </div>
      </div>
    </div>
  );
}

function bootstrap(config: { city?: string; units?: "celsius" | "fahrenheit" } | undefined) {
  document.documentElement.style.background = "transparent";
  document.body.style.margin = "0";
  document.body.style.background = "transparent";
  const root = createRoot(document.body);
  root.render(<App city={config?.city || "Paris"} units={config?.units || "celsius"} />);
}

if (window.evey) {
  window.evey.onInit((data) => bootstrap(data?.config));
} else {
  // Fallback when running outside the host bridge (dev preview).
  bootstrap(undefined);
}
