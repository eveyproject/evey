import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { router } from "expo-router";
import { getServerUrl } from "./server-config";
import { clearRefreshToken } from "./refresh-token";
import { forceRefresh } from "./auth-refresh";
import { eveyClientUserAgent } from "./clientIdentity";

export async function initFetcher() {
  const serverUrl = await getServerUrl();
  if (serverUrl) setBaseURL(serverUrl);
  // The User-Agent identifies this app (instead of okhttp's default) in the
  // admin logs and on the online panel - see clientIdentity.ts.
  setExtraHeaders({ "X-Persistent-Session": "true", "User-Agent": eveyClientUserAgent() });

  setTokenProvider(() => useAuthStore.getState().accessToken);
  setTokensUpdater((access) => useAuthStore.getState().setTokens(access));
  setLocaleProvider(() => useLocaleStore.getState().locale);

  setUnauthorizedHandler(() => {
    useAuthStore.getState().logout();
    clearRefreshToken();
    router.replace("/login");
  });

  // Route through forceRefresh so the bootstrap path, the AppState resume,
  // and 401 retries all coalesce on a single /auth/refresh round-trip.
  // Concurrent refreshes would trip server-side reuse detection and purge
  // every session for the user.
  setRefreshHandler(forceRefresh);
}
