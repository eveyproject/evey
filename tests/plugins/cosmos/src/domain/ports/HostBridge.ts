// Driven port: the plugin host (Evey iframe bridge). Shape mirrors
// weather-v2's HostBridge (onReady/resize/getPref/setPref/toast) - this
// plugin has no assets.read() use since every texture is procedural.

export interface HostInit {
  city: string;
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
  toast(level: "success" | "error" | "info", message: string): void;
}
