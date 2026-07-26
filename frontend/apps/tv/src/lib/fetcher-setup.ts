import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setLocaleProvider, setExtraHeaders } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { getServerUrl } from "./server-config";
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

  setRefreshHandler(async () => {
    const token = useAuthStore.getState().accessToken;
    if (!token) return null;
    const res = await fetch(`${serverUrl}/api/v1/auth/refresh`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Persistent-Session": "true",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({}),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { accessToken: string };
    return { accessToken: data.accessToken };
  });
}
