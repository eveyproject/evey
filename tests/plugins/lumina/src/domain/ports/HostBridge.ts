// Driven port: the plugin host (Evey iframe bridge). Shape mirrors
// weather-v2/cosmos's HostBridge (onReady/resize/getPref/setPref/toast) -
// this plugin has no assets.read() use since every visual is procedural.

import type { PresetId } from "../model/scene";

export interface HostInit {
  palette: PresetId;
  intensity: number; // 0..1
  speed: number; // 0.3..2
  locale: string;
  theme: string; // host theme mode ("dark" / "light"); drives settings-panel colors
}

export interface HostBridge {
  onReady(cb: (init: HostInit) => void): void;
  resize(heightPx: number): void;
  // Per-profile plugin preference store. getPref resolves null when
  // unset/unavailable so callers fall back to the admin config default.
  getPref(key: string): Promise<string | null>;
  setPref(key: string, value: string): Promise<void>;
  // Best-effort user feedback toast; no-op standalone.
  toast(level: "success" | "error" | "info", message: string): void;
}
