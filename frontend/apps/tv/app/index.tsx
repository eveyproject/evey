import { ActivityIndicator, View } from "react-native";
import { Redirect } from "expo-router";
import { useAuthStore } from "@evey/store";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { useTheme } from "../src/providers/ThemeProvider";

export default function EntryRedirect() {
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrlNullable();

  if (serverUrl === undefined) {
    return (
      <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  if (!serverUrl) return <Redirect href="/server-connect" />;
  if (!accessToken) return <Redirect href="/login" />;
  return <Redirect href="/(auth)/(tabs)" />;
}
