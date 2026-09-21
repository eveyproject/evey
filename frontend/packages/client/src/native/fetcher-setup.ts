import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders, resetRefreshFailure } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { forceRefresh } from "./auth-refresh";

export interface FetcherSetupOptions {
  /** Where the API lives. Resolved once at init; null leaves the base URL unset. */
  resolveServerUrl: () => Promise<string | null>;
  /** The product User-Agent (see each app's clientIdentity.ts). */
  userAgent: string;
  /**
   * What to do when the fetcher gives the session up. The apps differ on
   * purpose: a phone signs out and clears its caches; a television only
   * leaves for the login screen when the session is really gone, because a
   * viewer with no keyboard must not be ejected over one scoped 401.
   */
  onUnauthorized: () => void;
}

export async function initFetcher(options: FetcherSetupOptions) {
  const serverUrl = await options.resolveServerUrl();
  if (serverUrl) setBaseURL(serverUrl);
  // The User-Agent identifies this app (instead of okhttp's default) in the
  // admin logs and on the online panel.
  setExtraHeaders({ "X-Persistent-Session": "true", "User-Agent": options.userAgent });

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

  // Route through forceRefresh so the bootstrap path, the AppState resume,
  // and 401 retries all coalesce on a single /auth/refresh round-trip.
  // Concurrent refreshes would trip server-side reuse detection and purge
  // every session for the user.
  setRefreshHandler(forceRefresh);
}
