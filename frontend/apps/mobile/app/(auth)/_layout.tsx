import { useEffect, useState } from "react";
import { Redirect, Stack } from "expo-router";
import { useAuthStore, useServerListStore } from "@evey/store";
import { ensureAccessToken } from "../../src/lib/auth-refresh";
import { authGateDecision } from "../../src/lib/authGate";
import { useOnlineStatus } from "../../src/hooks/useOnlineStatus";
import { useProfileCacheSync } from "../../src/hooks/useProfileCacheSync";
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
import { DemoBanner } from "../../src/components/DemoBanner";

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
  // "pending" covers both the initial state and a refresh in flight - the tree
  // stays unrendered only for as long as we genuinely don't know yet.
  const [refreshState, setRefreshState] = useState<"pending" | "refused" | "unavailable">("pending");
  // Retrying the refresh the moment the link comes back is what turns "offline
  // mode" back into a live session without the user restarting the app.
  const online = useOnlineStatus();
  // Registers this app as a Play On target. No-ops internally while
  // accessToken/serverUrl aren't ready, so it's safe to call unconditionally
  // here (must be, since hooks can't sit after the early returns below).
  useDeviceRegistration();
  // Writes the four fields the cold-start picker draws itself from, whenever
  // the server's profile list lands. Here rather than on one screen so it
  // covers the whole session, and it costs no request - React Query dedupes it
  // against every other caller of the same list.
  useProfileCacheSync();
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
  // refresh and disconnected users mid-session. ensureAccessToken is
  // single-flight, so this coalesces with bootstrap/resume/401 refreshes.
  useEffect(() => {
    if (!expired) { setRefreshState("pending"); return; }
    let cancelled = false;
    setRefreshState("pending");
    void ensureAccessToken().then((state) => {
      if (cancelled) return;
      if (state === "refused") setRefreshState("refused");
      else if (state === "unavailable") setRefreshState("unavailable");
      // "valid"/"refreshed": a fresh token landed in the store and re-renders
      // us with expired=false, so there is nothing to latch.
    });
    return () => { cancelled = true; };
  }, [expired, accessToken, online]);

  // Only a REFUSAL ends the session; offline the tree renders at once so the
  // Downloads screen and local playback (which need no token) stay reachable
  // while the access token is aged out - see authGate.ts.
  const gate = authGateDecision({ accessToken, expired, refreshState, online, profileId });
  if (gate === "login") return <Redirect href="/login" />;
  if (gate === "wait") return null; // refresh in flight, online
  if (gate === "profile-select") return <Redirect href="/profile-select" />;

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
      <DemoBanner />
    </DrawerProvider>
  );
}
