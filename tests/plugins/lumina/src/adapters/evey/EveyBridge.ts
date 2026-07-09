// Adapter for the Evey iframe bridge (window.evey).
// Falls back to immediate defaults when running standalone (local dev).

import type { HostBridge, HostInit } from "../../domain/ports/HostBridge";
import { presetOrDefault } from "../../domain/services/presets";

interface EveyGlobal {
  onInit(cb: (data: {
    config?: { palette?: string; intensity?: number | string; speed?: number | string };
    locale?: string;
    theme?: string;
  }) => void): void;
  resize(height: number): void;
  toast(level: "success" | "error" | "info", message: string): void;
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

const DEFAULT_INTENSITY = 0.7;
const DEFAULT_SPEED = 1;
const INTENSITY_MIN = 0.2;
const INTENSITY_MAX = 1;
const SPEED_MIN = 0.3;
const SPEED_MAX = 2;

function toNumber(v: number | string | undefined, fallback: number, lo: number, hi: number): number {
  const n = typeof v === "number" ? v : typeof v === "string" ? Number(v) : NaN;
  return Number.isFinite(n) ? Math.min(hi, Math.max(lo, n)) : fallback;
}

export class EveyBridge implements HostBridge {
  onReady(cb: (init: HostInit) => void): void {
    const normalize = (
      config?: { palette?: string; intensity?: number | string; speed?: number | string },
      locale?: string,
      theme?: string,
    ): HostInit => ({
      palette: presetOrDefault(config?.palette),
      intensity: toNumber(config?.intensity, DEFAULT_INTENSITY, INTENSITY_MIN, INTENSITY_MAX),
      speed: toNumber(config?.speed, DEFAULT_SPEED, SPEED_MIN, SPEED_MAX),
      locale: locale ?? "en",
      theme: theme ?? "dark",
    });
    if (typeof window !== "undefined" && window.evey) {
      window.evey.onInit((data) => cb(normalize(data?.config, data?.locale, data?.theme)));
    } else {
      // Standalone dev: no host bridge.
      cb(normalize(undefined, navigator?.language, undefined));
    }
  }

  resize(heightPx: number): void {
    window.evey?.resize(Math.round(heightPx));
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
