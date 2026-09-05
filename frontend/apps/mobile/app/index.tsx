import { useEffect, useState } from "react";
import { ActivityIndicator, View } from "react-native";
import { Redirect } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { pingServer } from "../src/lib/server-config";
import { useTheme } from "../src/providers/ThemeProvider";

export default function EntryRedirect() {
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);
  const serverUrl = useServerUrlNullable();
  const [serverOk, setServerOk] = useState<boolean | undefined>(undefined);

  // A saved session skips the probe entirely (see the redirect below for why),
  // so don't spend a 5s round-trip - and a 5s spinner - on an answer nothing
  // reads.
  const hasSession = !!accessToken && !!profileId;

  useEffect(() => {
    if (!serverUrl || hasSession) return;
    pingServer(serverUrl).then(setServerOk);
  }, [serverUrl, hasSession]);

  // Still loading URL from SecureStore
  if (serverUrl === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  // No saved URL → server-connect
  if (!serverUrl) return <Redirect href="/server-connect" />;

  // An already-authenticated session goes straight in, reachable server or not.
  // The probe is a diagnosis for the CONNECT flow; as a GATE on a returning
  // user it was a dead end: offline (plane, tunnel, no data) it always fails,
  // and /server-connect cannot be completed without a server, so the app was
  // unusable exactly when its downloads were the point. Inside, every screen
  // has its own error state, the Downloads screen and local playback need no
  // server at all, and a genuinely moved server is still fixable from
  // Settings → the server list.
  if (hasSession) return <Redirect href="/(auth)/(tabs)" />;

  // URL saved, pinging server...
  if (serverOk === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  // Server unreachable → server-connect (will show saved URL)
  if (!serverOk) return <Redirect href="/server-connect" />;

  // Server OK but no auth → login
  if (!accessToken) return <Redirect href="/login" />;

  // Server OK + auth but no profile → profile select
  if (!profileId) return <Redirect href="/profile-select" />;

  // All good → home
  return <Redirect href="/(auth)/(tabs)" />;
}
