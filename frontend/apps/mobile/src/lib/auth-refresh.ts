import { useAuthStore } from "@evey/store";
import { getServerUrl } from "./server-config";
import { getRefreshToken, setRefreshToken, clearRefreshToken } from "./refresh-token";

function isAccessExpired(token: string | null): boolean {
  if (!token) return true;
  try {
    const parts = token.split(".");
    if (parts.length !== 3 || !parts[1]) return true;
    const payload = JSON.parse(
      atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")),
    ) as { exp?: number };
    if (typeof payload.exp !== "number") return true;
    // Treat tokens within 30s of expiry as expired so we don't race the server.
    return payload.exp < Date.now() / 1000 + 30;
  } catch {
    return true;
  }
}

// Single in-flight refresh. The server rotates and single-uses refresh
// tokens — two concurrent calls trigger reuse detection and PURGE every
// session for the user. Coalesce all refresh callers (bootstrap, AppState
// resume, 401 fetch retry) onto the same promise so only one /auth/refresh
// hits the wire per refresh window.
let inFlight: Promise<boolean> | null = null;

async function doRefresh(): Promise<boolean> {
  const auth = useAuthStore.getState();
  const stored = await getRefreshToken();
  if (!stored) {
    if (auth.accessToken) auth.logout();
    return false;
  }

  const base = await getServerUrl();
  if (!base) return false;

  try {
    const res = await fetch(`${base}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Persistent-Session": "true" },
      body: JSON.stringify({ refreshToken: stored }),
    });
    if (!res.ok) {
      await clearRefreshToken();
      auth.logout();
      return false;
    }
    const data = (await res.json()) as { accessToken: string; refreshToken?: string };
    if (data.refreshToken) await setRefreshToken(data.refreshToken);
    auth.setTokens(data.accessToken);
    return true;
  } catch {
    return false;
  }
}

// Returns true if a usable access token is present after the call. Non-throwing.
// Safe to invoke from any number of call sites — concurrent invocations
// share the same in-flight refresh.
export async function refreshAccessTokenIfNeeded(): Promise<boolean> {
  if (!isAccessExpired(useAuthStore.getState().accessToken)) return true;
  if (inFlight) return inFlight;
  inFlight = doRefresh();
  try {
    return await inFlight;
  } finally {
    inFlight = null;
  }
}

// Force a refresh regardless of whether the cached token *looks* valid.
// Used by the fetcher 401 handler — a 401 means the server already rejected
// our access token, so we must refresh even if its `exp` claim is in the
// future (e.g. signing-key rotation, server clock drift, server restart).
export async function forceRefresh(): Promise<{ accessToken: string } | null> {
  if (inFlight) {
    const ok = await inFlight;
    if (!ok) return null;
    const tok = useAuthStore.getState().accessToken;
    return tok ? { accessToken: tok } : null;
  }
  inFlight = doRefresh();
  try {
    const ok = await inFlight;
    if (!ok) return null;
    const tok = useAuthStore.getState().accessToken;
    return tok ? { accessToken: tok } : null;
  } finally {
    inFlight = null;
  }
}
