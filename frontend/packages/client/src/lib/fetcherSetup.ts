import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders, resetRefreshFailure } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";

export interface FetcherSetupOptions {
  /** Where the API lives. Resolved once at init; null leaves the base URL unset. */
  resolveServerUrl: () => Promise<string | null>;
  /** The product User-Agent (each app's clientIdentity). Omitted on a browser, which sets its own. */
  userAgent?: string;
  /**
   * What to do when the fetcher gives the session up. The apps differ on
   * purpose: a phone signs out and clears its caches; a television only
   * leaves for the login screen when the session is really gone, because a
   * viewer with no keyboard must not be ejected over one scoped 401.
   */
  onUnauthorized: () => void;
  /** The refresh the fetcher's 401 retry routes through (see createAuthRefresh). */
  forceRefresh: () => Promise<{ accessToken: string } | null>;
}

/**
 * Wires the API fetcher for a client that holds its own tokens: base URL,
 * bearer, locale, the refresh handler and the dead-session handler. Runs once
 * at startup, before the first query.
 */
export async function initFetcher(options: FetcherSetupOptions) {
  const serverUrl = await options.resolveServerUrl();
  if (serverUrl) setBaseURL(serverUrl);
  // X-Persistent-Session asks the server for a refresh token in the body
  // rather than an HttpOnly cookie: this client stores it itself. The
  // User-Agent identifies the app in the admin logs and on the online panel;
  // a browser refuses to override it and sends its own.
  setExtraHeaders(
    options.userAgent
      ? { "X-Persistent-Session": "true", "User-Agent": options.userAgent }
      : { "X-Persistent-Session": "true" },
  );

  setTokenProvider(() => useAuthStore.getState().accessToken);
  setTokensUpdater((access) => useAuthStore.getState().setTokens(access));
  setLocaleProvider(() => useLocaleStore.getState().locale);

  // A new access token arriving OUTSIDE the fetcher's own refresh (login,
  // boot-time refresh, AppState resume) starts a new session: the fetcher's
  // refresh-failure latch describes the previous one and must not survive it.
  useAuthStore.subscribe((state, prev) => {
    if (state.accessToken && state.accessToken !== prev.accessToken) resetRefreshFailure();
  });

  // Without this handler the fetcher's fallback is window.dispatchEvent,
  // which nothing listens to on a device - a dead session left the viewer on
  // a screen of failing queries with no way out.
  setUnauthorizedHandler(options.onUnauthorized);

  // Route through forceRefresh so the bootstrap path, the resume, and 401
  // retries all coalesce on a single /auth/refresh round-trip. Concurrent
  // refreshes would trip server-side reuse detection and purge every session
  // for the user.
  setRefreshHandler(options.forceRefresh);
}
