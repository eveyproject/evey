import { useAuthStore } from "@evey/store";
import { userServiceRefreshToken } from "@evey/api";

// A JWT access token counts as expired once its `exp` is in the past (or the
// token is unparseable). Shared by the route guards so index/login and the
// refresh path agree on when a token is dead.
export function isTokenExpired(token: string): boolean {
  try {
    const parts = token.split(".");
    if (parts.length < 3 || !parts[1]) return true;
    const payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.exp !== "number" || payload.exp * 1000 < Date.now();
  } catch {
    return true;
  }
}

// Runs fn while holding the cross-tab exclusive Web Lock so two tabs never POST
// /auth/refresh at the same time. Falls back to running directly where the Web
// Locks API is unavailable.
async function underRefreshLock<T>(fn: () => Promise<T>): Promise<T> {
  if (typeof navigator !== "undefined" && (navigator as Navigator & { locks?: LockManager | null }).locks) {
    return (navigator as Navigator & { locks: LockManager }).locks.request(
      "evey-refresh-token",
      { mode: "exclusive" },
      fn,
    ) as Promise<T>;
  }
  return fn();
}

// Refreshes the access token under the cross-tab lock. The refresh endpoint
// authenticates via the single HttpOnly refresh cookie, so two tabs whose tokens
// expired together (e.g. after sleep) POSTing concurrently trips server-side
// reuse-detection and purges EVERY session. The route guards used to call the
// refresh RPC directly, bypassing the lock the proactive timer already uses;
// this helper is the single locked path for both. If another tab refreshed
// while we waited on the lock, its fresh token is reused instead of POSTing
// again. Returns true when a usable (non-expired) token is held afterwards.
export async function refreshTokenUnderLock(): Promise<boolean> {
  return underRefreshLock(async () => {
    const current = useAuthStore.getState().accessToken;
    if (current && !isTokenExpired(current)) return true;
    try {
      const data = await userServiceRefreshToken({});
      if (data?.accessToken) {
        useAuthStore.getState().setTokens(data.accessToken);
        return true;
      }
      return false;
    } catch (err) {
      console.warn("token refresh failed", err);
      return false;
    }
  });
}
