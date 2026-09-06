import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders, resetRefreshFailure } from "@evey/api";
import { useAuthStore, useLocaleStore, useServerListStore, clearProfileCache } from "@evey/store";
import { router } from "expo-router";
import { resolveServerUrl } from "./server-config";
import { clearRefreshToken } from "./refresh-token";

import { forceRefresh } from "./auth-refresh";
import { eveyClientUserAgent } from "./clientIdentity";

export async function initFetcher() {
  // Same resolution as the boot and every screen: active server first, legacy
  // key as the fallback (see resolveServerUrl for what divergence cost us).
  const serverUrl = await resolveServerUrl(useServerListStore.getState().getActiveServer()?.url);
  if (serverUrl) setBaseURL(serverUrl);
  // The User-Agent identifies this app (instead of okhttp's default) in the
  // admin logs and on the online panel - see clientIdentity.ts.
  setExtraHeaders({ "X-Persistent-Session": "true", "User-Agent": eveyClientUserAgent() });

  setTokenProvider(() => useAuthStore.getState().accessToken);
  setTokensUpdater((access) => useAuthStore.getState().setTokens(access));
  setLocaleProvider(() => useLocaleStore.getState().locale);

  // A new access token arriving OUTSIDE the fetcher's own refresh (login,
  // AppState resume refresh) starts a new session: the fetcher's
  // refresh-failure latch describes the previous one and must not survive it.
  useAuthStore.subscribe((state, prev) => {
    if (state.accessToken && state.accessToken !== prev.accessToken) resetRefreshFailure();
  });

  setUnauthorizedHandler(() => {
    useAuthStore.getState().logout();
    clearRefreshToken();
    // The cached profile list describes ONE account on ONE server. Left behind,
    // it would name the previous household's profiles on the next login.
    void clearProfileCache();
    router.replace("/login");
  });

  // Route through forceRefresh so the bootstrap path, the AppState resume,
  // and 401 retries all coalesce on a single /auth/refresh round-trip.
  // Concurrent refreshes would trip server-side reuse detection and purge
  // every session for the user.
  setRefreshHandler(forceRefresh);
}
