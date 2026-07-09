// Adapter for the Evey iframe bridge (window.evey).
// Falls back to immediate defaults when running standalone (local dev).

import type { HostBridge, HostInit } from "../../domain/ports/HostBridge";
import type { MediaSummary } from "../../domain/model/media";

interface RawPublicMediaItem {
  id?: string;
  title?: string;
  year?: number;
  posterUrl?: string;
}

interface RawListResponse {
  items?: RawPublicMediaItem[];
}

interface EveyGlobal {
  onInit(cb: (data: { locale?: string; theme?: string }) => void): void;
  resize(height: number): void;
  navigate(path: string): void;
  media?: {
    list(opts?: { libraryId?: string }): Promise<unknown>;
    get(id: string): Promise<unknown>;
    poster?(id: string): Promise<unknown>;
  };
}

declare global {
  interface Window {
    evey?: EveyGlobal;
  }
}

function toMediaSummary(raw: RawPublicMediaItem): MediaSummary | null {
  if (!raw || typeof raw.id !== "string" || !raw.id) return null;
  if (typeof raw.posterUrl !== "string" || !raw.posterUrl) return null;
  return {
    id: raw.id,
    title: typeof raw.title === "string" ? raw.title : "",
    year: typeof raw.year === "number" && raw.year > 0 ? raw.year : undefined,
    posterUrl: raw.posterUrl,
  };
}

export class EveyBridge implements HostBridge {
  onReady(cb: (init: HostInit) => void): void {
    const normalize = (locale?: string, theme?: string): HostInit => ({
      locale: locale ?? "en",
      theme: theme ?? "dark",
    });
    if (typeof window !== "undefined" && window.evey) {
      window.evey.onInit((data) => cb(normalize(data?.locale, data?.theme)));
    } else {
      // Standalone dev: no host bridge.
      cb(normalize(navigator?.language, undefined));
    }
  }

  resize(heightPx: number): void {
    window.evey?.resize(Math.round(heightPx));
  }

  navigate(path: string): void {
    window.evey?.navigate(path);
  }

  async readPoster(id: string): Promise<string> {
    const api = window.evey?.media;
    if (!api?.poster || !id) return "";
    try {
      const url = await api.poster(id);
      return typeof url === "string" ? url : "";
    } catch {
      return "";
    }
  }

  async listMedia(): Promise<MediaSummary[]> {
    const api = window.evey?.media;
    if (!api?.list) return [];
    try {
      const res = (await api.list({})) as RawListResponse | RawPublicMediaItem[] | null;
      const items = Array.isArray(res) ? res : Array.isArray(res?.items) ? res.items : [];
      const out: MediaSummary[] = [];
      for (const raw of items) {
        const summary = toMediaSummary(raw);
        if (summary) out.push(summary);
      }
      return out;
    } catch {
      return [];
    }
  }
}
