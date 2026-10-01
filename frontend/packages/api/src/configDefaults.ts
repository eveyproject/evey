import { eveyFetch } from "./fetcher";

// What the language-and-region settings fall back to when left empty, as the
// server resolves them: the panel shows "Default: French", not "env default".
export interface ConfigDefaults {
  "server.language"?: string;
  "metadata.language"?: string;
  "server.timezone"?: string;
  /** The server's clock when it answered (RFC 3339). */
  now?: string;
}

export const configDefaultsKey = ["/api/v1/admin/config/defaults"] as const;

export function getConfigDefaults(signal?: AbortSignal) {
  return eveyFetch<ConfigDefaults>({ url: "/api/v1/admin/config/defaults", method: "GET", signal });
}
