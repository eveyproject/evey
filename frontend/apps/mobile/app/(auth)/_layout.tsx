import { useEffect, useState } from "react";
import { Redirect, Stack } from "expo-router";
import { useAuthStore, useServerListStore } from "@evey/store";
import { refreshAccessTokenIfNeeded } from "../../src/lib/auth-refresh";
import { initI18n } from "../../src/i18n";
import { resolveServerUrl } from "../../src/lib/server-config";
import { useDeviceRegistration } from "../../src/hooks/useDeviceRegistration";
import { usePresenceHeartbeat } from "../../src/hooks/usePresenceHeartbeat";
import { DrawerProvider } from "../../src/providers/DrawerProvider";
import { SideDrawer } from "../../src/components/SideDrawer";
import { MiniPlayer } from "../../src/components/MiniPlayer";
import { MusicMiniPlayer } from "../../src/components/MusicMiniPlayer";
import { OfflineBanner } from "../../src/components/OfflineBanner";
import { ImpersonationBanner } from "../../src/components/ImpersonationBanner";

function isTokenExpired(token: string): boolean {
  try {
    const tokenParts = token.split(".");
    if (tokenParts.length < 3 || !tokenParts[1]) return true;
    const payload = JSON.parse(atob(tokenParts[1].replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.exp === "number" && payload.exp < Date.now() / 1000;
  } catch {
    return true;
  }
}

export default function AuthLayout() {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const expired = !!accessToken && isTokenExpired(accessToken);
  const [refreshFailed, setRefreshFailed] = useState(false);
  // Registers this app as a Play On target. No-ops internally while
  // accessToken/serverUrl aren't ready, so it's safe to call unconditionally
  // here (must be, since hooks can't sit after the early returns below).
  useDeviceRegistration();
  // Second chance for the translation catalogue: a boot whose single i18n
  // fetch failed (server asleep, flaky link) otherwise renders raw keys for
  // the WHOLE session. initI18n no-ops when the catalogue already arrived.
  useEffect(() => {
    void (async () => {
      const url = await resolveServerUrl(useServerListStore.getState().getActiveServer()?.url);
      if (url) await initI18n(url);
    })();
  }, []);
  // Foreground presence beats for the admin "online" panel - enabled only
  // once fully authenticated, for the same hook-order reason as above.
  usePresenceHeartbeat(!!accessToken && !expired && !!profileId);

  // An expired ACCESS token is not a logout: a valid refresh token usually
  // still sits in SecureStore (access TTL is 15 min). Swap it for a fresh JWT
  // instead of bouncing to /login - logging out here raced every re-render of
  // this layout (navigation, rotation, resume) against the fetchers' own
  // refresh and disconnected users mid-session. refreshAccessTokenIfNeeded is
  // single-flight, so this coalesces with bootstrap/resume/401 refreshes.
  useEffect(() => {
    if (!expired) return;
    let cancelled = false;
    void refreshAccessTokenIfNeeded().then((ok) => {
      if (!cancelled && !ok) setRefreshFailed(true);
    });
    return () => { cancelled = true; };
  }, [expired, accessToken]);

  if (!accessToken) return <Redirect href="/login" />;
  if (expired) {
    if (refreshFailed) return <Redirect href="/login" />;
    return null; // refresh in flight; setTokens re-renders us with the new JWT
  }
  if (!profileId) return <Redirect href="/profile-select" />;

  return (
    <DrawerProvider>
      <Stack screenOptions={{ headerShown: false }}>
        <Stack.Screen name="profile/manage" options={{ headerShown: true, title: "" }} />
        <Stack.Screen name="profile/settings" options={{ headerShown: true, title: "" }} />
        <Stack.Screen name="manage-servers" options={{ headerShown: true, title: "" }} />
      </Stack>
      <SideDrawer />
      <MiniPlayer />
      <MusicMiniPlayer />
      <OfflineBanner />
      <ImpersonationBanner />
    </DrawerProvider>
  );
}
