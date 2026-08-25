import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders, resetRefreshFailure } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { router } from "expo-router";
import { getServerUrl } from "./server-config";
import { eveyClientUserAgent } from "./clientIdentity";
import { forceRefresh } from "./auth-refresh";

export async function initFetcher() {
  const serverUrl = await getServerUrl();
  if (serverUrl) setBaseURL(serverUrl);
  // The User-Agent identifies this app (instead of okhttp's default) in the
  // admin logs and on the online panel - see clientIdentity.ts.
  setExtraHeaders({ "X-Persistent-Session": "true", "User-Agent": eveyClientUserAgent() });

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
  // which nothing listens to on TV - a dead session left the viewer on a
  // screen of failing queries with no way out. Only route to login when the
  // session is actually gone (doRefresh clears the tokens on refusal); a 401
  // scoped to one call while a token is still held must not eject a viewer
  // who has no keyboard to log back in with.
  setUnauthorizedHandler(() => {
    if (useAuthStore.getState().accessToken) return;
    router.replace("/login");
  });

  // Route through forceRefresh: it uses the stored refresh token (the access
  // token is already rejected at this point) and coalesces concurrent callers
  // so the server's single-use rotation never sees the same token twice.
  setRefreshHandler(forceRefresh);
}
