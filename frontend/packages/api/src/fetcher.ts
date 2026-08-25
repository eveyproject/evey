// Custom fetch wrapper used by all orval-generated hooks
// Injects Authorization header from the auth store automatically

let getToken: (() => string | null) | null = null;
let updateTokens: ((access: string) => void) | null = null;
let getLocale: (() => string) | null = null;
export function setLocaleProvider(fn: () => string) { getLocale = fn; }

let baseURL = "";
export function setBaseURL(url: string) { baseURL = url; }
export function getBaseURL(): string { return baseURL; }

let extraHeaders: Record<string, string> = {};
export function setExtraHeaders(headers: Record<string, string>) { extraHeaders = headers; }

let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) { onUnauthorized = fn; }

type RefreshFn = () => Promise<{ accessToken: string } | null>;
let customRefreshFn: RefreshFn | null = null;
export function setRefreshHandler(fn: RefreshFn) { customRefreshFn = fn; }

function getCSRFToken(): string {
  if (typeof document === "undefined") return "";
  const match = document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/);
  return match ? match[1] : "";
}

export function setTokenProvider(fn: () => string | null) { getToken = fn; }
export function setTokensUpdater(fn: (access: string) => void) {
  updateTokens = (token) => { refreshFailed = false; fn(token); };
}

export interface FetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  data?: unknown;
  params?: Record<string, unknown>;
  signal?: AbortSignal;
}

let refreshPromise: Promise<string> | null = null;
let refreshFailed = false;

/** Reset module state - for tests only */
export function _resetFetcherState() { refreshPromise = null; refreshFailed = false; }

/**
 * Lift the refresh-failure latch. App wiring calls this whenever a NEW access
 * token lands in the auth store outside the fetcher's own refresh path (login,
 * impersonation, a boot-time refresh that bypasses this module) - the latch
 * describes the PREVIOUS session being dead and must never outlive it. Without
 * this, a re-login after a revoked session inherited the latch and was thrown
 * back to the login screen on the first token expiry.
 */
export function resetRefreshFailure() { refreshFailed = false; }

// Treat JWTs as expired when within this many seconds of their `exp` claim.
// Buffers clock skew + the time it takes to round-trip a request.
const EXPIRY_GRACE_SECONDS = 5;

/**
 * Decode a JWT payload (no signature verify) and return whether it's
 * already expired (or close enough that we should refresh now).
 *
 * Returns true on any parse failure - better to attempt a refresh than
 * to send a malformed token.
 */
function isJwtExpired(token: string): boolean {
  const parts = token.split(".");
  if (parts.length !== 3) return false; // not a JWT we recognize - trust caller
  try {
    const payload = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = payload + "=".repeat((4 - (payload.length % 4)) % 4);
    // atob is available in browsers, RN Hermes, and Node ≥ 18. Buffer is
    // Node-only and missing from TS 6's default lib for browser/RN targets,
    // so don't reference it - every runtime we ship to has atob.
    const json = atob(padded);
    const claims = JSON.parse(json) as { exp?: number };
    if (typeof claims.exp !== "number") return false;
    const nowSeconds = Math.floor(Date.now() / 1000);
    return claims.exp <= nowSeconds + EXPIRY_GRACE_SECONDS;
  } catch {
    return false; // unparseable JWT - let the server reject with 401
  }
}

/**
 * Resolve the access token to use for an outbound request. If the cached
 * token is expired (or missing on a request that previously had one),
 * await the single-flight refresh so all concurrent callers reuse the
 * same fresh token instead of each tripping a 401 + their own retry.
 *
 * Exported for the test suite.
 */
export async function _ensureFreshToken(url?: string): Promise<string | undefined> {
  const token = getToken?.() ?? undefined;
  // Never pre-refresh before hitting the refresh endpoint itself: it
  // authenticates via the HttpOnly cookie, not the Bearer, so triggering
  // attemptRefresh here would rotate the cookie once, then the explicit
  // /auth/refresh POST rotates it again - a double rotation that needlessly
  // doubles the multi-tab reuse-detection surface (which purges all sessions).
  if (url && url.endsWith("/auth/refresh")) return token;
  if (!token) return undefined;
  if (!isJwtExpired(token)) return token;
  if (refreshFailed) return token;       // give up - caller will get 401 → fireUnauthorized
  try {
    return await attemptRefresh();
  } catch {
    return token;                        // fall through; the 401 path will fire unauthorized
  }
}

function resolveURL(url: string): string {
  return baseURL ? `${baseURL}${url}` : url;
}

function fireUnauthorized() {
  if (onUnauthorized) { onUnauthorized(); return; }
  if (typeof window !== "undefined") window.dispatchEvent(new Event("evey:unauthorized"));
}

async function attemptRefresh(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      if (customRefreshFn) {
        const result = await customRefreshFn();
        if (!result) throw Object.assign(new Error("refresh failed"), { httpStatus: 401 });
        updateTokens?.(result.accessToken);
        return result.accessToken;
      }
      const csrfToken = getCSRFToken();
      const refreshHeaders: Record<string, string> = { "Content-Type": "application/json" };
      if (csrfToken) refreshHeaders["X-CSRF-Token"] = csrfToken;
      const res = await fetch(resolveURL("/api/v1/auth/refresh"), {
        method: "POST",
        credentials: "include",
        headers: refreshHeaders,
        body: JSON.stringify({}),
      });
      if (!res.ok) throw Object.assign(new Error("refresh failed"), { httpStatus: res.status });
      const data = await res.json() as { accessToken: string };
      updateTokens?.(data.accessToken);
      return data.accessToken;
    })().finally(() => { refreshPromise = null; });
  }
  return refreshPromise;
}

export interface RawFetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  body?: BodyInit;
  signal?: AbortSignal;
  // authToken overrides the Authorization header with an explicit Bearer token
  // (e.g. a scoped plugin token). When set, the user-token refresh-on-401 path
  // is skipped - the caller owns this token's lifecycle.
  authToken?: string;
  // raw: return the Response even on a non-2xx status instead of throwing, so
  // the caller can branch on res.status/body (e.g. 428 escalation, 403 blocked).
  // 401 still goes through the token-refresh/unauthorized flow.
  raw?: boolean;
  // cache: forwarded to fetch() as RequestCache. "no-cache" forces conditional
  // revalidation against stored entries - needed for stable-name resources
  // (plugin bundles) where an old cache entry may predate the server's
  // no-cache+ETag policy and would otherwise be served stale without a request.
  cache?: RequestCache;
}

async function fetchWithRefresh(
  url: string,
  doFetch: (authToken: string | undefined) => Promise<Response>,
  initialToken: string | undefined,
  rawOnError = false,
): Promise<Response> {
  let response = await doFetch(initialToken);

  if (response.status === 401 && !url.includes("/auth/refresh") && !url.includes("/auth/logout")) {
    if (refreshFailed) {
      fireUnauthorized();
      throw Object.assign(new Error("Unauthorized"), { httpStatus: 401 });
    }
    try {
      const newToken = await attemptRefresh();
      refreshFailed = false;
      response = await doFetch(newToken);
    } catch (refreshErr) {
      // Only a REFUSAL - the server rejecting the refresh credential itself -
      // means the session is dead: latch so we stop hammering the endpoint,
      // and tell the app so it can route to login. Anything else (network
      // down, a proxy 502, the NAS rebooting) is TRANSIENT: a later call must
      // be free to refresh again, and nobody gets logged out for it. Latching
      // on transients left long-lived clients (a TV stays on for days)
      // permanently unable to refresh after a single hiccup.
      const status = (refreshErr as { httpStatus?: number }).httpStatus;
      if (status === 401 || status === 403) {
        refreshFailed = true;
        fireUnauthorized();
      }
      throw Object.assign(new Error("Unauthorized"), { httpStatus: 401 });
    }
  }

  if (response.status === 401) {
    fireUnauthorized();
  }

  // rawOnError: hand the caller the Response even on non-2xx (except 401, which
  // still triggers the refresh/unauthorized flow above) so it can inspect
  // res.status/body itself - e.g. a 428 permission-escalation or 403 blocked
  // response whose body carries structured fields the caller must act on.
  if (rawOnError) {
    return response;
  }

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    let retryAfterSeconds: number | undefined;
    let errorCode: string | undefined;
    try {
      const body = await response.clone().json() as { message?: string; retryAfterSeconds?: number; code?: unknown };
      if (body.message) message = body.message;
      if (typeof body.retryAfterSeconds === "number") retryAfterSeconds = body.retryAfterSeconds;
      // A server-supplied machine code beats prose: the UI can translate a known
      // cause instead of showing an English sentence from the API. Only string
      // codes count - gRPC-gateway errors put a NUMBER in `code`.
      if (typeof body.code === "string" && body.code !== "") errorCode = body.code;
    } catch {
      // ignore parse errors
    }
    // The remote-access gate refuses every off-LAN client on every route. Its
    // header survives a body a proxy rewrote, and without this signal the app
    // showed a generic failure while the real cause was one server toggle
    // (2026-07-29 lockout: two days of diagnosis).
    if (response.headers.get("X-Evey-Remote-Access") === "disabled") {
      errorCode = "remote_access_disabled";
    }
    // A 429 must reach the UI with its WAIT attached: "too many requests" on
    // its own is unactionable and the user just retries into the wall
    // (2026-07-29). Header wins, body is the fallback for stripping proxies.
    const header = Number.parseInt(response.headers.get("Retry-After") ?? "", 10);
    if (Number.isFinite(header) && header > 0) retryAfterSeconds = header;
    throw Object.assign(new Error(message), { httpStatus: response.status, retryAfterSeconds, errorCode });
  }

  return response;
}

export async function eveyFetchRaw(request: RawFetcherRequest): Promise<Response> {
  const method = request.method.toUpperCase();

  // Explicit-token path (e.g. a scoped plugin token): plain fetch, no
  // user-token refresh. Returns the raw Response (caller inspects res.ok) so a
  // scope denial (403) surfaces to the caller rather than triggering a refresh.
  if (request.authToken) {
    const h: Record<string, string> = {
      ...extraHeaders,
      ...request.headers,
      Authorization: `Bearer ${request.authToken}`,
    };
    const loc = getLocale?.();
    if (loc) h["Accept-Language"] = loc;
    if (method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE") {
      const csrf = getCSRFToken();
      if (csrf) h["X-CSRF-Token"] = csrf;
    }
    return fetch(resolveURL(request.url), {
      method: request.method,
      credentials: "include" as const,
      headers: h,
      body: request.body,
      signal: request.signal,
      ...(request.cache ? { cache: request.cache } : {}),
    });
  }

  const token = await _ensureFreshToken(request.url);
  const headers: Record<string, string> = { ...extraHeaders, ...request.headers };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const locale = getLocale?.();
  if (locale) headers["Accept-Language"] = locale;
  if (method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE") {
    const csrf = getCSRFToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  const fullURL = resolveURL(request.url);
  const doFetch = (authToken: string | undefined) =>
    fetch(fullURL, {
      method: request.method,
      credentials: "include" as const,
      headers: authToken ? { ...headers, Authorization: `Bearer ${authToken}` } : headers,
      body: request.body,
      signal: request.signal,
      ...(request.cache ? { cache: request.cache } : {}),
    });

  return fetchWithRefresh(fullURL, doFetch, token ?? undefined, request.raw ?? false);
}

export async function eveyFetch<T>(request: FetcherRequest): Promise<T> {
  const token = await _ensureFreshToken(request.url);
  const headers: Record<string, string> = { ...extraHeaders, ...request.headers };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const locale = getLocale?.();
  if (locale) headers["Accept-Language"] = locale;
  const method = request.method.toUpperCase();
  if (method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE") {
    const csrf = getCSRFToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }
  if (request.data !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  let url = resolveURL(request.url);
  if (request.params) {
    const entries: [string, string][] = [];
    for (const [k, v] of Object.entries(request.params)) {
      if (v === undefined || v === null) continue;
      if (Array.isArray(v)) {
        for (const item of v) entries.push([k, String(item)]);
      } else {
        entries.push([k, String(v)]);
      }
    }
    const qs = new URLSearchParams(entries).toString();
    if (qs) url = `${url}?${qs}`;
  }

  const doFetch = (authToken: string | undefined) =>
    fetch(url, {
      method: request.method,
      credentials: "include" as const,
      headers: authToken ? { ...headers, Authorization: `Bearer ${authToken}` } : headers,
      body: request.data !== undefined ? JSON.stringify(request.data) : undefined,
      signal: request.signal,
    });

  const response = await fetchWithRefresh(url, doFetch, token ?? undefined);

  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }

  // grpc-gateway server-streaming responses are NDJSON (one JSON object per line).
  // Parse the last non-empty line to handle both regular and streaming responses.
  const text = await response.text();
  const lines = text.split("\n").filter((l) => l.trim());
  if (lines.length === 0) return undefined as T;
  return JSON.parse(lines[lines.length - 1]) as T;
}
