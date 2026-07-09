// Driven port: the plugin host (Evey iframe bridge).

import type { MediaSummary } from "../model/media";

export interface HostInit {
  locale: string;
  theme: string; // host theme mode ("dark" / "light")
}

export interface HostBridge {
  onReady(cb: (init: HostInit) => void): void;
  resize(heightPx: number): void;
  navigate(path: string): void;
  // Reads the caller's library, gated server-side to the safe `public`
  // projection (id/title/year/posterUrl only). Resolves an empty array on
  // any error so callers degrade to an idle/empty wall instead of throwing.
  listMedia(): Promise<MediaSummary[]>;
  // Resolves a media item's poster as a compact data: URL served by the host
  // (the sandbox CSP forbids the external CDN URL and its opaque origin taints
  // cross-origin textures, so this host-relayed data URL is the only way to
  // paint library posters). Resolves "" on any failure so the tile keeps its
  // placeholder gradient.
  readPoster(id: string): Promise<string>;
}
