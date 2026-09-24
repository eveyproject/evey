import { setBaseURL, setCredentialsMode } from "@evey/api";
import { useAuthStore, useServerListStore, normalizeServerUrl } from "@evey/store";
import { createAuthRefresh, initFetcher, parseEveyHealth, type EveyHealth } from "@evey/client";
import { reloadCatalogue } from "../i18n";
import { servingServer } from "./servedServer";

// What this client keeps on the television: the server it talks to and the
// refresh token it holds itself (no HttpOnly cookie in a packaged app, and
// the same code path under /tv keeps one behaviour). localStorage is the one
// store a TV browser offers; a private window loses it and the viewer signs
// in again, which is the honest outcome.
const REFRESH_KEY = "evey-tv-refresh-token";

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    /* storage refused */
  }
}

export const refreshTokenStore = {
  getRefreshToken: async () => read(REFRESH_KEY),
  setRefreshToken: async (token: string) => write(REFRESH_KEY, token),
  clearRefreshToken: async () => write(REFRESH_KEY, null),
};

export function activeServerUrl(): string | null {
  return useServerListStore.getState().getActiveServer()?.url ?? null;
}

export const authRefresh = createAuthRefresh({
  ...refreshTokenStore,
  getServerUrl: async () => activeServerUrl(),
});

/**
 * Points the API at a server. The server list store is the same one the
 * phone and the television use (persisted by zustand in localStorage here),
 * so a server added once is offered again at the next boot.
 */
export function selectServer(url: string, name?: string): void {
  const normalized = normalizeServerUrl(url);
  const store = useServerListStore.getState();
  const existing = Object.values(store.servers).find((s) => normalizeServerUrl(s.url) === normalized);
  if (existing) {
    store.setActiveServer(existing.id);
    if (name) store.setServerName(existing.id, name);
  } else {
    const added = store.addServer(normalized);
    store.setActiveServer(added.id);
    if (name) store.setServerName(added.id, name);
  }
  setBaseURL(normalized);
  reloadCatalogue();
}

export type ServerProbe = "ok" | "not-evey" | "unreachable";

/**
 * Is an EVEY server reachable at url - not merely "does something answer
 * 200 there" (a captive portal, a router page). Same rule as the phone and
 * the television, through the shared health parser.
 */
export async function probeServer(url: string, timeoutMs = 5000): Promise<{ probe: ServerProbe; health: EveyHealth | null }> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${normalizeServerUrl(url)}/healthz`, { signal: controller.signal });
    if (!res.ok) return { probe: "unreachable", health: null };
    const body: unknown = await res.json().catch(() => null);
    const health = parseEveyHealth(body);
    return health ? { probe: "ok", health } : { probe: "not-evey", health: null };
  } catch {
    return { probe: "unreachable", health: null };
  } finally {
    clearTimeout(timeout);
  }
}

export async function signOut(): Promise<void> {
  const access = useAuthStore.getState().accessToken;
  const stored = await refreshTokenStore.getRefreshToken();
  const base = activeServerUrl();
  if (access && base) {
    void fetch(`${base}/api/v1/auth/logout`, {
      method: "POST",
      // No cookie: under /tv the server's _csrf cookie would get this POST
      // refused (see authRefreshCore); the session is named in the body.
      credentials: "omit",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${access}` },
      body: JSON.stringify(stored ? { refreshToken: stored } : {}),
    }).catch(() => {});
  }
  await refreshTokenStore.clearRefreshToken();
  useAuthStore.getState().logout();
}

/** Wires the API fetcher once, at boot, and answers whether a server is known. */
export async function bootSession(onUnauthorized: () => void): Promise<{ hasServer: boolean }> {
  // First visit of a page served by an Evey server: that server is the one.
  if (!activeServerUrl()) {
    const own = await servingServer(window.location, probeServer);
    if (own) selectServer(own);
  }
  const url = activeServerUrl();
  // Bearer only, never cookies: this client may run from another origin (a
  // packaged app, the /tv page opened on a different host) and the server
  // grants a foreign origin no credentials.
  setCredentialsMode("omit");
  await initFetcher({
    resolveServerUrl: async () => url,
    onUnauthorized,
    forceRefresh: authRefresh.forceRefresh,
  });
  // A server remembered from the last visit: its catalogue, or every screen
  // stays on the embedded English.
  if (url) reloadCatalogue();
  return { hasServer: !!url };
}
