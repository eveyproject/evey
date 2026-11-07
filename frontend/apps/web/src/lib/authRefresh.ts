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
  return (await refreshTokenWithOutcome()) === "ok";
}

// "denied" is the server's word (401/403: the session is dead) and the only
// outcome that justifies signing the user out. "transient" is the network or
// the server being unavailable: the session may well be alive, and the route
// guard used to log out on it all the same - a flaky link cost the session.
export type RefreshOutcome = "ok" | "denied" | "transient";

export async function refreshTokenWithOutcome(): Promise<RefreshOutcome> {
  const outcome = await underRefreshLock(async (): Promise<RefreshOutcome> => {
    const current = useAuthStore.getState().accessToken;
    if (current && !isTokenExpired(current)) return "ok";
    try {
      const data = await userServiceRefreshToken({});
      if (data?.accessToken) {
        useAuthStore.getState().setTokens(data.accessToken);
        return "ok";
      }
      return "denied";
    } catch (err) {
      console.warn("token refresh failed", err);
      const status = (err as { httpStatus?: number }).httpStatus;
      // Only the server's own refusal of the cookie outlives this document: a
      // 403 is the gate in front of it (the CSRF guard, remote access off).
      if (status === 401) noteSessionEnded();
      return status === 401 || status === 403 ? "denied" : "transient";
    }
  });
  if (outcome === "denied") refusedSinceLiveToken = true;
  return outcome;
}

// Whether this document has held a live token, and whether a refresh was
// refused since. A /login arrival on a document that held one follows a
// sign-out, whose revocation may still be in flight; one after a refusal was
// answered already: the form stands. A document that never held one is a page
// load, and the refresh cookie may still be honoured - asking for a password
// there is asking a signed-in visitor to sign in.
let heldLiveToken = false;
let refusedSinceLiveToken = false;

// What this browser last knew of its refresh cookie, which the page cannot
// read. "ended" is written only when the server itself ended the cookie: a
// sign-out, or a refresh it refused. Anything else, the absence included,
// means the cookie may be live and the server is the one to ask - an
// "ended" guessed from a missing hint would show the form on a live session.
const SESSION_HINT_KEY = "evey-session-hint";

function writeSessionHint(value: "live" | "ended"): void {
  try {
    localStorage.setItem(SESSION_HINT_KEY, value);
  } catch { /* storage unavailable: the server is asked, as before */ }
}

function sessionKnownEnded(): boolean {
  try {
    return localStorage.getItem(SESSION_HINT_KEY) === "ended";
  } catch {
    return false;
  }
}

// Reported by the app's store watcher (main.tsx) for every live token the
// document takes on, the persisted one included. A live token was issued
// with the cookie, so the cookie is live too.
export function noteLiveTokenHeld(): void {
  heldLiveToken = true;
  refusedSinceLiveToken = false;
  writeSessionHint("live");
}

// The server ended this browser's cookie: a sign-out it was asked for, or a
// refresh it refused. Until a sign-in lands, no /login load asks it again.
export function noteSessionEnded(): void {
  writeSessionHint("ended");
}

// The login route asking whether a password is needed at all. True when a
// usable token is held afterwards; false, with no request, when the session
// is known to be over.
export async function renewOnLoginArrival(): Promise<boolean> {
  if (heldLiveToken || refusedSinceLiveToken || sessionKnownEnded()) return false;
  return refreshTokenUnderLock();
}
