// Driven port: the plugin host (Evey iframe bridge).

import type { CinephileItem } from "../model/media";

export interface HostInit {
  locale: string;
  theme: string; // host theme mode ("dark" / "light")
}

export interface HostBridge {
  onReady(cb: (init: HostInit) => void): void;
  resize(heightPx: number): void;
  navigate(path: string): void;
  // Reads the caller's library, gated server-side to the safe `public`
  // projection (id/type/title/year/posterUrl only). Resolves an empty array
  // on any error so callers degrade to an idle/empty galaxy instead of
  // throwing.
  listMedia(): Promise<CinephileItem[]>;
}
