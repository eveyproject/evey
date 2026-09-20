// Driven port: the plugin host (Evey iframe bridge).

import type { Units } from "../model/weather";

export interface HostInit {
  city: string;
  units: Units;
  locale: string;
  theme: string; // host theme mode ("dark" / "light"); drives settings-panel colors
  // The height the host recommends for this slot on the surface it put us on
  // (CSS px). Absent on a host too old to say.
  suggestedHeight?: number;
}

export interface HostBridge {
  onReady(cb: (init: HostInit) => void): void;
  resize(heightPx: number): void;
  // Reads one of the plugin's own web_root assets as a data: URL (e.g. a sun
  // image the backend downloaded at runtime). Resolves null when unavailable
  // (no host bridge, asset missing, or any error) so callers fall back.
  readAsset(path: string): Promise<string | null>;
  // Per-profile plugin preference store (scoped host-side to the active
  // profile). getPref resolves null when unset/unavailable so callers fall back
  // to the admin config default.
  getPref(key: string): Promise<string | null>;
  setPref(key: string, value: string): Promise<void>;
  // Best-effort user feedback toast; no-op standalone.
  toast(level: "success" | "error" | "info", message: string): void;
}
