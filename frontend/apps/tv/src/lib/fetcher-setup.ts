import { setBaseURL, setTokenProvider, setTokensUpdater, setRefreshHandler } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getServerUrl } from "./server-config";

export async function initFetcher() {
  const serverUrl = await getServerUrl();
  if (serverUrl) setBaseURL(serverUrl);

  setTokenProvider(() => useAuthStore.getState().accessToken);
  setTokensUpdater((access) => useAuthStore.getState().setTokens(access));

  setRefreshHandler(async () => {
    const token = useAuthStore.getState().accessToken;
    if (!token) return null;
    const res = await fetch(`${serverUrl}/api/v1/auth/refresh`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({}),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as { accessToken: string };
    return { accessToken: data.accessToken };
  });
}
