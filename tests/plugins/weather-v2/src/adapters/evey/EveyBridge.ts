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
  }) => void): void;
  resize(height: number): void;
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  assets?: { read(path: string): Promise<string> };
}

declare global {
  interface Window {
    evey?: EveyGlobal;
  }
}

export class EveyBridge implements HostBridge {
  onReady(cb: (init: HostInit) => void): void {
    const normalize = (config?: { city?: string; units?: Units }, locale?: string): HostInit => ({
      city: config?.city ?? "Paris",
      units: config?.units === "fahrenheit" ? "fahrenheit" : "celsius",
      locale: locale ?? "en",
    });
    if (typeof window !== "undefined" && window.evey) {
      window.evey.onInit((data) => cb(normalize(data?.config, data?.locale)));
    } else {
      // Standalone dev: no host bridge.
      cb(normalize(undefined, navigator?.language));
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
}
