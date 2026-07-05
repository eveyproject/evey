// Driven port: the plugin host (Evey iframe bridge).

import type { Units } from "../model/weather";

export interface HostInit {
  city: string;
  units: Units;
  locale: string;
}

export interface HostBridge {
  onReady(cb: (init: HostInit) => void): void;
  resize(heightPx: number): void;
  // Reads one of the plugin's own web_root assets as a data: URL (e.g. a sun
  // image the backend downloaded at runtime). Resolves null when unavailable
  // (no host bridge, asset missing, or any error) so callers fall back.
  readAsset(path: string): Promise<string | null>;
}
