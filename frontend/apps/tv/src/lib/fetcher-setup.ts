import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setLocaleProvider, setExtraHeaders } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
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

  // Route through forceRefresh: it uses the stored refresh token (the access
  // token is already rejected at this point) and coalesces concurrent callers
  // so the server's single-use rotation never sees the same token twice.
  setRefreshHandler(forceRefresh);
}
