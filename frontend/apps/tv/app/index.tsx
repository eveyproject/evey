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

  if (serverUrl === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (!serverUrl) return <Redirect href="/server-connect" />;

  if (serverOk === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  // Say WHY: an unreachable server used to drop the viewer on a screen that
  // looks exactly like a first-time setup, with no hint that the server they
  // already saved is simply asleep.
  if (!serverOk) return <Redirect href="/server-connect?unreachable=1" />;
  if (!accessToken) return <Redirect href="/login" />;
  if (!profileId) return <Redirect href="/profile-select" />;
  return <Redirect href="/(auth)/(tabs)" />;
}
