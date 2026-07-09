// Adapter for the Evey iframe bridge (window.evey).
// Falls back to immediate defaults when running standalone (local dev).

import type { HostBridge, HostInit } from "../../domain/ports/HostBridge";
import type { CinephileItem, MediaKind } from "../../domain/model/media";

interface RawPublicMediaItem {
  id?: string;
  type?: string;
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
  };
}

declare global {
  interface Window {
    evey?: EveyGlobal;
  }
}

function toMediaKind(raw?: string): MediaKind {
  if (raw === "MEDIA_ITEM_TYPE_MOVIE") return "movie";
  if (raw === "MEDIA_ITEM_TYPE_SERIES") return "series";
  return "other";
}

function toCinephileItem(raw: RawPublicMediaItem): CinephileItem | null {
  if (!raw || typeof raw.id !== "string" || !raw.id) return null;
  if (typeof raw.title !== "string" || !raw.title) return null;
  return {
    id: raw.id,
    title: raw.title,
    year: typeof raw.year === "number" && raw.year > 0 ? raw.year : undefined,
    posterUrl: typeof raw.posterUrl === "string" && raw.posterUrl ? raw.posterUrl : undefined,
    type: toMediaKind(raw.type),
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

  async listMedia(): Promise<CinephileItem[]> {
    const api = window.evey?.media;
    if (!api?.list) return [];
    try {
      const res = (await api.list({})) as RawListResponse | RawPublicMediaItem[] | null;
      const items = Array.isArray(res) ? res : Array.isArray(res?.items) ? res.items : [];
      const out: CinephileItem[] = [];
      for (const raw of items) {
        const item = toCinephileItem(raw);
        if (item) out.push(item);
      }
      return out;
    } catch {
      return [];
    }
  }
}
