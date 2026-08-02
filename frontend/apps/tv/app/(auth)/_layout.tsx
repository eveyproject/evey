import { useEffect, useState } from "react";
import { ActivityIndicator, View } from "react-native";
import { Redirect, Stack, router, usePathname } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useDeviceRegistration } from "../../src/hooks/useDeviceRegistration";
import { usePresenceHeartbeat } from "../../src/hooks/usePresenceHeartbeat";
import { TVMusicPlayer } from "../../src/components/TVMusicPlayer";
import { HomeShortcutButton } from "../../src/components/HomeShortcutButton";
import { isDeepScreenPath, HOME_PATH } from "../../src/lib/homeShortcut";
import { ErrorBoundary } from "../../src/components/ErrorBoundary";
import { refreshAccessTokenIfNeeded } from "../../src/lib/auth-refresh";
import { useTheme } from "../../src/providers/ThemeProvider";

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
  const theme = useTheme();
  const [renewalDone, setRenewalDone] = useState(false);
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const logout = useAuthStore((s) => s.logout);
  const pathname = usePathname();
  // Register this client as a Play On target. No-ops when not
  // authenticated (the hook bails if accessToken/serverUrl are absent),
  // so calling it before the redirect-on-no-auth guard is safe.
  useDeviceRegistration();
  // Foreground presence beats for the admin "online" panel - enabled only
  // once fully authenticated (hooks can't sit after the guards below).
  usePresenceHeartbeat(!!accessToken && !isTokenExpired(accessToken) && !!profileId);

  // An expired access token is not a logged-out viewer: the refresh token
  // outlives it by design. Try the silent renewal before sending a TV back to
  // a password form it has no keyboard for.
  const needsRenewal = !!accessToken && isTokenExpired(accessToken);
  useEffect(() => {
    if (!needsRenewal) return;
    let cancelled = false;
    void refreshAccessTokenIfNeeded().then((ok) => {
      if (cancelled) return;
      if (!ok) logout();
      setRenewalDone(true);
    });
    return () => { cancelled = true; };
  }, [needsRenewal, logout]);

  if (needsRenewal && !renewalDone) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (!accessToken || isTokenExpired(accessToken)) {
    return <Redirect href="/login" />;
  }
  if (!profileId) return <Redirect href="/profile-select" />;

  return (
    <View style={{ flex: 1 }}>
      {/* A boundary HERE, not only at the root: one at the root turns any
          render error into a blank application, taking the music player and
          the way home with it. Contained at this level, a broken screen shows
          its own recovery card while the shell around it keeps working. */}
      <ErrorBoundary>
        <Stack screenOptions={{ headerShown: false }} />
      </ErrorBoundary>
      {isDeepScreenPath(pathname) && (
        <HomeShortcutButton onPress={() => router.replace(HOME_PATH as never)} />
      )}
      <TVMusicPlayer />
    </View>
  );
}
