// Main React component: transparent R3F Canvas + HTML overlay.
import { Suspense } from "react";
import { Canvas } from "@react-three/fiber";
import type { SceneParams } from "./domain/weather";
import { SceneContext } from "./scene/WeatherScene";
import { Clouds } from "./scene/Clouds";
import { Precipitation } from "./scene/Precipitation";
import { CelestialBody } from "./scene/CelestialBody";
import { translate } from "./i18n/labels";

interface AppProps {
  params: SceneParams;
  city: string;
  temp: number | null;
  units: "celsius" | "fahrenheit";
  error: string | null;
  locale: string;
}

export function App({ params, city, temp, units, error, locale }: AppProps) {
  const unitLabel      = units === "fahrenheit" ? "°F" : "°C";
  const tempStr        = temp !== null ? `${Math.round(temp)}${unitLabel}` : "—";
  const conditionLabel = translate(params.labelKey, locale);
  const loadingLabel   = translate("loading", locale);
  const unavailLabel   = translate("unavailable", locale);

  return (
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "280px",
        borderRadius: "16px",
        overflow: "hidden",
        background: "transparent",
      }}
    >
      <Canvas
        camera={{ position: [0, 0, 4], fov: 40, near: 0.1, far: 50 }}
        dpr={[1, 1.5]}
        gl={{
          antialias: true,
          alpha: true,
          premultipliedAlpha: false,
        }}
        style={{
          position: "absolute",
          inset: 0,
          width: "100%",
          height: "100%",
        }}
      >
        <SceneContext.Provider value={params}>
          <Suspense fallback={null}>
            {/* Background TRANSPARENT — no opaque skydome. Clouds composite
            *  over whatever the host page shows. CelestialBody rendered last
            *  at high renderOrder so it sits above the cloud layer. */}
            <Clouds />
            <Precipitation />
            <CelestialBody />
          </Suspense>
        </SceneContext.Provider>
      </Canvas>

      {/* HTML overlay — top-right, always on top of canvas */}
      <div
        style={{
          position: "absolute",
          top: 16,
          right: 20,
          color: "#ffffff",
          fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
          textAlign: "right",
          pointerEvents: "none",
          userSelect: "none",
        }}
      >
        <div
          style={{
            fontSize: 44,
            fontWeight: 600,
            letterSpacing: "-0.02em",
            lineHeight: 1,
            textShadow: "0 2px 8px rgba(0,0,0,0.55), 0 1px 3px rgba(0,0,0,0.35)",
          }}
        >
          {error ? "—" : tempStr}
        </div>
        <div
          style={{
            fontSize: 13,
            opacity: 0.88,
            marginTop: 6,
            textTransform: "uppercase",
            letterSpacing: "0.08em",
            textShadow: "0 1px 4px rgba(0,0,0,0.60)",
          }}
        >
          {error
            ? unavailLabel
            : temp !== null
              ? `${city} · ${conditionLabel}`
              : loadingLabel}
        </div>
      </div>
    </div>
  );
}
