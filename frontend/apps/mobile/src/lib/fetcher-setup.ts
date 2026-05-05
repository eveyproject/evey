import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { router } from "expo-router";
import { getServerUrl } from "./server-config";
import { clearRefreshToken } from "./refresh-token";
import { forceRefresh } from "./auth-refresh";

export async function initFetcher() {
  const serverUrl = await getServerUrl();
  if (serverUrl) setBaseURL(serverUrl);
  setExtraHeaders({ "X-Persistent-Session": "true" });

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
