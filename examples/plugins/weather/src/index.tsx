// Entry point for the weather plugin front module.
//
// Bridge contract: window.evey.onInit fires with { config: { city, units } }.
// Flow: onInit → geocode + fetchWeather → conditionToScene → render App.
//
// Open-Meteo endpoints (whitelisted in plugin.yaml):
//   https://geocoding-api.open-meteo.com/v1/search
//   https://api.open-meteo.com/v1/forecast

import { createRoot } from "react-dom/client";
import { useState, useEffect, StrictMode } from "react";
import { OpenMeteoWeatherProvider } from "./adapters/OpenMeteoWeatherProvider";
import { conditionToScene } from "./domain/mapping/conditionToScene";
import type { SceneParams, WeatherSnapshot } from "./domain/weather";
import { App } from "./App";

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

const provider = new OpenMeteoWeatherProvider();

// Placeholder scene for loading state (clear day)
const LOADING_PARAMS: SceneParams = {
  cloudCover: 0,
  cloudDensity: 0,
  cloudSpeed: 0.1,
  precipitation: "none",
  precipIntensity: 0,
  windVec: [1, 0],
  thunderRate: 0,
  fog: 0,
  isDay: false,
  sunAltitude: -0.4,
  sunAzimuth: 0,
  moonPhase: 0.5,
  palette: {
    zenith:  [0.18, 0.42, 0.82],
    horizon: [0.85, 0.72, 0.52],
    sun:     [1.00, 0.92, 0.65],
    fog:     [0.95, 0.92, 0.88],
  },
  label: "Loading…",
  labelKey: "loading",
};

interface PluginState {
  params: SceneParams;
  snap: WeatherSnapshot | null;
  error: string | null;
}

function WeatherPlugin({ city, units, locale }: { city: string; units: "celsius" | "fahrenheit"; locale: string }) {
  const [state, setState] = useState<PluginState>({
    params: LOADING_PARAMS,
    snap: null,
    error: null,
  });

  // Signal host of desired iframe height once.
  useEffect(() => {
    window.evey?.resize(280);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const coord = await provider.geocode(city, locale);
        const current = await provider.getCurrent(coord, units);
        if (cancelled) return;
        const snap: WeatherSnapshot = {
          ...current,
          cityName: coord.name,
          units,
        };
        const params = conditionToScene(snap, new Date());
        setState({ params, snap, error: null });
      } catch (e) {
        if (cancelled) return;
        setState(prev => ({
          ...prev,
          error: e instanceof Error ? e.message : "fetch failed",
        }));
      }
    })();
    return () => { cancelled = true; };
  }, [city, units]);

  return (
    <App
      params={state.params}
      city={state.snap?.cityName ?? city}
      temp={state.snap?.temperatureDisplay ?? null}
      units={units}
      error={state.error}
      locale={locale}
    />
  );
}

function bootstrap(
  config: { city?: string; units?: "celsius" | "fahrenheit" } | undefined,
  locale: string | undefined,
) {
  document.documentElement.style.background = "transparent";
  document.body.style.margin = "0";
  document.body.style.background = "transparent";
  document.body.style.overflow = "hidden";

  const root = createRoot(document.body);
  root.render(
    <StrictMode>
      <WeatherPlugin
        city={config?.city ?? "Paris"}
        units={config?.units ?? "celsius"}
        locale={locale ?? "en"}
      />
    </StrictMode>
  );
}

if (typeof window !== "undefined" && window.evey) {
  window.evey.onInit((data) => bootstrap(data?.config, data?.locale));
} else {
  bootstrap(undefined, undefined);
}
