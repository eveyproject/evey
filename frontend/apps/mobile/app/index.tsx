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

  useEffect(() => {
    if (!serverUrl) return;
    pingServer(serverUrl).then(setServerOk);
  }, [serverUrl]);

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
