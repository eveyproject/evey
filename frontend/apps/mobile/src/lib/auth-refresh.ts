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
// tokens - two concurrent calls on the same token trigger reuse detection
// (the second one finds the row gone after the first claim+delete) and the
// caller is logged out. Coalesce all refresh callers (bootstrap, AppState
// resume, 401 fetch retry) onto the same promise so only one /auth/refresh
// hits the wire per refresh window.
let inFlight: Promise<RefreshOutcome> | null = null;

// "refused" = the server rejected the refresh credential: the session is dead
// and the caller may treat it as such. "unavailable" = we could not get an
// answer (network down, proxy 5xx, server rebooting): the session is NOT dead
// and nothing downstream may latch, log out, or clear tokens over it. The old
// boolean collapsed both - a proxy 502 during a token expiry logged the user
// out and burned their (still valid) refresh token.
type RefreshOutcome = "ok" | "refused" | "unavailable";

async function doRefresh(): Promise<RefreshOutcome> {
  const auth = useAuthStore.getState();
  const stored = await getRefreshToken();
  if (!stored) {
    if (auth.accessToken) auth.logout();
    return "refused";
  }

  const base = await getServerUrl();
  if (!base) return "unavailable";

  try {
    const res = await fetch(`${base}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Persistent-Session": "true" },
      body: JSON.stringify({ refreshToken: stored }),
    });
    // Only a REFUSAL ends the session; any other non-ok answer is the
    // infrastructure, not the credential, and must leave the session intact.
    if (res.status === 401 || res.status === 403) {
      await clearRefreshToken();
      auth.logout();
      return "refused";
    }
    if (!res.ok) return "unavailable";
    const data = (await res.json()) as { accessToken?: string; refreshToken?: string };
    // A 200 carrying no token is not a success: overwriting the live token with
    // undefined would log the user out just as surely, only more quietly.
    if (!data.accessToken) return "unavailable";
    if (data.refreshToken) await setRefreshToken(data.refreshToken);
    auth.setTokens(data.accessToken);
    return "ok";
  } catch {
    return "unavailable";
  }
}

// Returns true if a usable access token is present after the call. Non-throwing.
// Safe to invoke from any number of call sites - concurrent invocations
// share the same in-flight refresh.
export async function refreshAccessTokenIfNeeded(): Promise<boolean> {
  if (!isAccessExpired(useAuthStore.getState().accessToken)) return true;
  if (inFlight) return (await inFlight) === "ok";
  inFlight = doRefresh();
  try {
    return (await inFlight) === "ok";
  } finally {
    inFlight = null;
  }
}

// Force a refresh regardless of whether the cached token *looks* valid.
// Used by the fetcher 401 handler - a 401 means the server already rejected
// our access token, so we must refresh even if its `exp` claim is in the
// future (e.g. signing-key rotation, server clock drift, server restart).
// Contract with the fetcher's refresh handler: `null` means REFUSED (dead
// session - the fetcher latches and fires unauthorized), a THROW means
// transient unavailability (the fetcher stays free to retry later).
export async function forceRefresh(): Promise<{ accessToken: string } | null> {
  if (!inFlight) inFlight = doRefresh();
  try {
    const outcome = await inFlight;
    if (outcome === "refused") return null;
    if (outcome !== "ok") throw new Error("refresh unavailable: no answer from the server");
    const tok = useAuthStore.getState().accessToken;
    return tok ? { accessToken: tok } : null;
  } finally {
    inFlight = null;
  }
}
