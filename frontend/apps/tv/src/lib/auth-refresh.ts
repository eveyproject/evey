import { useAuthStore } from "@evey/store";
import { getServerUrl } from "./server-config";
import { getRefreshToken, setRefreshToken, clearRefreshToken } from "./refresh-token";

export function isAccessExpired(token: string | null): boolean {
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

// Single in-flight refresh. The server rotates and single-uses refresh tokens -
// two concurrent calls on the same token trigger reuse detection and log the
// viewer out. A TV boots several screens at once (layout guard, device
// registration, presence), so coalescing matters more here than on mobile.
let inFlight: Promise<RefreshOutcome> | null = null;

// "refused" = the server rejected the refresh credential: the session is dead
// and the caller may treat it as such. "unavailable" = we could not get an
// answer (network down, proxy 5xx, NAS rebooting): the session is NOT dead and
// nothing downstream may latch or log out over it. Collapsing both into
// `false` is what used to freeze the fetcher's refresh latch forever after a
// single hiccup on a TV that stays on for days.
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
    // Only a REFUSAL ends the session. A television is left on for days and
    // will meet its server restarting, a proxy hiccuping, a 502 from a NAS
    // waking up: logging the viewer out on those means they come back to a
    // login screen and no keyboard to answer it with.
    if (res.status === 401 || res.status === 403) {
      await clearRefreshToken();
      auth.logout();
      return "refused";
    }
    if (!res.ok) return "unavailable";

    const data = (await res.json()) as { accessToken?: string; refreshToken?: string };
    // A 200 carrying no token is not a success: overwriting the live token with
    // undefined would log the viewer out just as surely, only more quietly.
    if (!data.accessToken) return "unavailable";
    if (data.refreshToken) await setRefreshToken(data.refreshToken);
    auth.setTokens(data.accessToken);
    return "ok";
  } catch {
    return "unavailable";
  }
}

// Returns true if a usable access token is present after the call. Non-throwing.
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

// Force a refresh regardless of whether the cached token looks valid: a 401
// means the server already rejected it, whatever its exp claim says.
//
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
