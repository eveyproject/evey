import { useAuthStore } from "@evey/store";
import { clientLogger } from "../log";

const log = clientLogger("auth");

// The refresh-token flow every client that holds its refresh token itself
// runs: the phone and the television in their secure store, the browser TV
// client in its local storage. The web app is different - its refresh token
// is an HttpOnly cookie - and keeps its own path.

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

// "refused" = the server rejected the refresh credential: the session is dead
// and the caller may treat it as such. "unavailable" = we could not get an
// answer (network down, proxy 5xx, server rebooting): the session is NOT dead
// and nothing downstream may latch, log out, or clear tokens over it. The old
// boolean collapsed both - a proxy 502 during a token expiry logged the user
// out and burned their (still valid) refresh token.
export type RefreshOutcome = "ok" | "refused" | "unavailable";

/**
 * What the app may conclude about the session right now.
 *
 * A boolean would collapse "refused" and "unavailable" into one `false`,
 * which is exactly the confusion RefreshOutcome exists to prevent: a caller
 * that ROUTES on the answer (the (auth) layout redirecting to /login) must be
 * able to tell a dead session from a dead link. Offline with a valid refresh
 * token still in the store is a supported mode - downloaded titles play there
 * - and must never end at the login screen.
 */
export type AccessTokenState = "valid" | "refreshed" | "refused" | "unavailable";

export interface RefreshTokenStorage {
  getRefreshToken(): Promise<string | null>;
  setRefreshToken(token: string): Promise<void>;
  clearRefreshToken(): Promise<void>;
  /** The server to refresh against; null when none is configured yet. */
  getServerUrl(): Promise<string | null>;
}

// A refresh that never answers must not hold the boot - or the (auth) layout,
// which renders nothing while one is in flight - hostage. An unreachable but
// ROUTABLE host (captive portal, NAS asleep on a reachable LAN, phone on a
// data plan that resolves but drops) leaves fetch pending for the OS connect
// timeout, tens of seconds of blank screen. Offline is a supported mode, so
// give up early and answer "unavailable": the session is not dead, the link is.
const REFRESH_TIMEOUT_MS = 8000;

// True only for a token whose exp claim is in the past: an unreadable token
// is the server's business, not a late answer.
function expiredByClaim(token: string): boolean {
  try {
    const payload = JSON.parse(atob((token.split(".")[1] ?? "").replace(/-/g, "+").replace(/_/g, "/"))) as { exp?: unknown };
    return typeof payload.exp === "number" && payload.exp < Date.now() / 1000;
  } catch {
    return false;
  }
}

export interface AuthRefresh {
  ensureAccessToken(): Promise<AccessTokenState>;
  refreshAccessTokenIfNeeded(): Promise<boolean>;
  forceRefresh(): Promise<{ accessToken: string } | null>;
  renewOnSignInScreen(): Promise<boolean>;
}

/**
 * Builds the refresh flow over the given storage. One instance per app:
 * every caller (bootstrap, resume, the fetcher's 401 retry) shares its single
 * in-flight refresh, because the server rotates and single-uses refresh
 * tokens - two concurrent calls on the same token trigger reuse detection
 * (the second finds the row gone after the first claim+delete) and the
 * caller is logged out.
 */
export function createAuthRefresh(storage: RefreshTokenStorage): AuthRefresh {
  let inFlight: Promise<RefreshOutcome> | null = null;
  // Set when the last refresh was refused by the server's limiter (429).
  let limited: { httpStatus: number; retryAfterSeconds?: number } | null = null;
  // Whether this process has held a live access token. A sign-in screen
  // reached after one follows a sign-out, whose revocation may still be in
  // flight, or a refusal, which already spoke: the form stands. One reached
  // without any is a boot whose renewal got no answer, and the stored refresh
  // token may still be honoured.
  let heldLiveToken = !isAccessExpired(useAuthStore.getState().accessToken);
  useAuthStore.subscribe((s) => {
    if (!isAccessExpired(s.accessToken)) heldLiveToken = true;
  });

  async function doRefresh(lateAnswerRetried = false): Promise<RefreshOutcome> {
    const auth = useAuthStore.getState();
    // The keystore read can reject (Android SecureStore's DecryptException).
    // Thrown from here it escaped the outcome entirely: the mobile gate waited
    // on "pending" forever (a blank shell) and the TV spinner never stopped.
    // An unreadable store is an absent answer, not a refusal - the session
    // stays as it is.
    let stored: string | null;
    let base: string | null;
    try {
      stored = await storage.getRefreshToken();
      if (!stored) {
        if (auth.accessToken) {
          log.warn("session ended: no refresh token in the store");
          auth.logout();
        }
        return "refused";
      }
      base = await storage.getServerUrl();
    } catch (err) {
      console.warn("auth refresh: secure storage read failed", err);
      return "unavailable";
    }
    if (!base) return "unavailable";

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), REFRESH_TIMEOUT_MS);
    try {
      const res = await fetch(`${base}/api/v1/auth/refresh`, {
        method: "POST",
        // The refresh token travels in the body: no cookie is needed, and one
        // must not ride along. Served by the Evey server itself (/tv), the page
        // holds the server's _csrf cookie, and a POST carrying it without the
        // matching header is refused 403 - which this flow reads as a dead
        // session: every access-token expiry (15 min) signed the viewer out
        // (production logs 2026-09-24, reproduced on a same-origin page).
        credentials: "omit",
        headers: { "Content-Type": "application/json", "X-Persistent-Session": "true" },
        body: JSON.stringify({ refreshToken: stored }),
        signal: controller.signal,
      });
      // Only a REFUSAL ends the session; any other non-ok answer is the
      // infrastructure, not the credential, and must leave the session intact.
      // The server refuses a credential with 401 alone: a 403 on this call is
      // the gate in front of it (remote access switched off while the phone is
      // away from home, the CSRF guard, a bouncer banning the household's
      // address) - read as a refusal it burned the refresh token of every
      // device for a ban that lifted an hour later.
      if (res.status === 401) {
        log.warn("session ended: refresh answered 401");
        await storage.clearRefreshToken();
        auth.logout();
        return "refused";
      }
      if (res.status === 403) {
        const gate = res.headers.get("X-Evey-Remote-Access");
        log.warn(`refresh unavailable: answered 403${gate ? ` (remote access ${gate})` : ""}`);
        return "unavailable";
      }
      if (!res.ok) {
        if (res.status === 429) {
          const retryAfter = Number.parseInt(res.headers.get("Retry-After") ?? "", 10);
          limited = { httpStatus: 429, ...(Number.isFinite(retryAfter) ? { retryAfterSeconds: retryAfter } : {}) };
        }
        log.warn(`refresh unavailable: answered ${res.status}`);
        return "unavailable";
      }
      const data = (await res.json()) as { accessToken?: string; refreshToken?: string };
      // A 200 carrying no token is not a success: overwriting the live token with
      // undefined would log the user out just as surely, only more quietly.
      if (!data.accessToken) return "unavailable";
      if (data.refreshToken) await storage.setRefreshToken(data.refreshToken);
      // A television that goes to sleep with this request in flight reads the
      // answer only on wake, hours later: its access token expired long ago.
      // Held, it reads as a renewal that left the session expired and the
      // screen guard signs the viewer out. The rotated refresh token above is
      // live: renew once more with it instead.
      if (expiredByClaim(data.accessToken)) {
        log.warn("refresh answer arrived already expired: renewing again");
        if (lateAnswerRetried) return "unavailable";
        clearTimeout(timer);
        return doRefresh(true);
      }
      auth.setTokens(data.accessToken);
      return "ok";
    } catch (err) {
      // Includes the abort above: a timed-out refresh is an absent answer, not a
      // rejected credential.
      log.warn(`refresh unavailable: ${controller.signal.aborted ? "timed out" : (err as Error)?.message ?? "network error"}`);
      return "unavailable";
    } finally {
      clearTimeout(timer);
    }
  }

  // Coalesces every caller onto one round-trip and clears the slot exactly
  // once, in the initiating call.
  async function runRefresh(): Promise<RefreshOutcome> {
    if (inFlight) return inFlight;
    inFlight = doRefresh();
    try {
      return await inFlight;
    } finally {
      inFlight = null;
    }
  }

  async function ensureAccessToken(): Promise<AccessTokenState> {
    if (!isAccessExpired(useAuthStore.getState().accessToken)) return "valid";
    const outcome = await runRefresh();
    if (outcome === "ok") return "refreshed";
    return outcome === "refused" ? "refused" : "unavailable";
  }

  // Returns true if a usable access token is present after the call. Non-throwing.
  // Callers that must distinguish "the server said no" from "nobody answered"
  // use ensureAccessToken instead.
  async function refreshAccessTokenIfNeeded(): Promise<boolean> {
    const state = await ensureAccessToken();
    return state === "valid" || state === "refreshed";
  }

  // Force a refresh regardless of whether the cached token *looks* valid.
  // Used by the fetcher 401 handler - a 401 means the server already rejected
  // our access token, so we must refresh even if its `exp` claim is in the
  // future (e.g. signing-key rotation, server clock drift, server restart).
  // Contract with the fetcher's refresh handler: `null` means REFUSED (dead
  // session - the fetcher latches and fires unauthorized), a THROW means
  // transient unavailability (the fetcher stays free to retry later).
  async function forceRefresh(): Promise<{ accessToken: string } | null> {
    limited = null;
    const outcome = await runRefresh();
    if (outcome === "refused") return null;
    // The limiter's answer keeps its status and delay, so the fetcher relays a
    // 429 the caller can back off by instead of an authorization failure.
    if (outcome !== "ok" && limited) throw Object.assign(new Error("refresh limited"), limited);
    if (outcome !== "ok") throw new Error("refresh unavailable: no answer from the server");
    const tok = useAuthStore.getState().accessToken;
    return tok ? { accessToken: tok } : null;
  }

  // The sign-in screen asking whether a password is needed at all. True when
  // a live token is held afterwards; false, with no request, on a process
  // that already had its session.
  async function renewOnSignInScreen(): Promise<boolean> {
    if (heldLiveToken) return false;
    return refreshAccessTokenIfNeeded();
  }

  return { ensureAccessToken, refreshAccessTokenIfNeeded, forceRefresh, renewOnSignInScreen };
}
