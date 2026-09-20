// Adapter for the Evey iframe bridge (window.evey).
// Falls back to immediate defaults when running standalone (local dev).

import type { HostBridge, HostInit } from "../../domain/ports/HostBridge";
import type { Units } from "../../domain/model/weather";

interface EveyGlobal {
  onInit(cb: (data: {
    context?: Record<string, unknown>;
    config?: { city?: string; units?: Units };
    locale?: string;
    theme?: string;
    surface?: string;
    viewportHeight?: number;
  }) => void): void;
  resize(height: number): void;
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  assets?: { read(path: string): Promise<string> };
  prefs?: {
    get(key: string): Promise<{ value?: string; found?: boolean } | string | null>;
    set(key: string, value: string): Promise<unknown>;
  };
}

declare global {
  interface Window {
    evey?: EveyGlobal;
  }
}

export class EveyBridge implements HostBridge {
  onReady(cb: (init: HostInit) => void): void {
    const normalize = (config?: { city?: string; units?: Units }, locale?: string, theme?: string, surface?: string, viewportHeight?: number): HostInit => ({
      city: config?.city ?? "Paris",
      units: config?.units === "fahrenheit" ? "fahrenheit" : "celsius",
      locale: locale ?? "en",
      theme: theme ?? "dark",
      surface,
      viewportHeight,
    });
    if (typeof window !== "undefined" && window.evey) {
      window.evey.onInit((data) => cb(normalize(data?.config, data?.locale, data?.theme, data?.surface, data?.viewportHeight)));
    } else {
      // Standalone dev: no host bridge.
      cb(normalize(undefined, navigator?.language, undefined));
    }
  }

  resize(heightPx: number): void {
    window.evey?.resize(Math.round(heightPx));
  }

  async readAsset(path: string): Promise<string | null> {
    const api = window.evey?.assets;
    if (!api?.read) return null;
    try {
      const dataUrl = await api.read(path);
      return typeof dataUrl === "string" && dataUrl.startsWith("data:") ? dataUrl : null;
    } catch {
      return null;
    }
  }

  async getPref(key: string): Promise<string | null> {
    const api = window.evey?.prefs;
    if (!api?.get) return null;
    try {
      const r = await api.get(key);
      if (r == null) return null;
      if (typeof r === "string") return r;
      // GetPref RPC shape: { value, found }.
      if (r.found === false) return null;
      return typeof r.value === "string" ? r.value : null;
    } catch {
      return null;
    }
  }

  async setPref(key: string, value: string): Promise<void> {
    const api = window.evey?.prefs;
    if (!api?.set) return;
    try {
      await api.set(key, value);
    } catch {
      /* best-effort */
    }
  }

  toast(level: "success" | "error" | "info", message: string): void {
    window.evey?.toast?.(level, message);
  }
}
