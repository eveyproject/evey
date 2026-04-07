import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler, setUnauthorizedHandler, setLocaleProvider, setExtraHeaders } from "@evey/api";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { router } from "expo-router";
import { getServerUrl } from "./server-config";
import { getRefreshToken, setRefreshToken, clearRefreshToken } from "./refresh-token";

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

  setRefreshHandler(async () => {
    const refreshToken = await getRefreshToken();
    if (!refreshToken) return null;
    const base = await getServerUrl();
    if (!base) return null;
    const res = await fetch(`${base}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Persistent-Session": "true" },
      body: JSON.stringify({ refreshToken }),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { accessToken: string; refreshToken?: string };
    if (data.refreshToken) await setRefreshToken(data.refreshToken);
    return { accessToken: data.accessToken };
  });
}
