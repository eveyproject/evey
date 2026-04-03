import { useState, useMemo } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { router } from "expo-router";
import { setServerUrl } from "../src/lib/server-config";
import { initFetcher } from "../src/lib/fetcher-setup";
import { initI18n } from "../src/i18n";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ServerConnect() {
  const theme = useTheme();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);

  const inputStyle = useMemo(() => ({
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
  }), [theme]);

  const containerStyle = useMemo(() => ({
    flex: 1,
    justifyContent: "center" as const,
    padding: theme.spacing.xl,
    backgroundColor: theme.colors.bg,
  }), [theme]);

  const buttonStyle = useMemo(() => ({
    backgroundColor: theme.colors.accent,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    alignItems: "center" as const,
  }), [theme]);

  async function handleConnect() {
    const trimmed = url.trim().replace(/\/+$/, "");
    if (!trimmed) return;

    setLoading(true);
    try {
      const res = await fetch(`${trimmed}/healthz`);
      if (!res.ok) throw new Error(`Server returned ${res.status}`);
      await setServerUrl(trimmed);
      await initFetcher();
      await initI18n(trimmed);
      router.replace("/login");
    } catch (e) {
      Alert.alert(
        "Connection failed",
        e instanceof Error ? e.message : "Unable to reach server",
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <View style={containerStyle}>
      <Text style={{ fontSize: 36, fontWeight: "800", color: theme.colors.accent, marginBottom: 8, letterSpacing: -1 }}>
        Evey
      </Text>
      <Text style={[scStyles.heading, { color: theme.colors.text, marginBottom: 4, fontSize: 18 }]}>
        Connect to your server
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: theme.spacing.lg }}>
        Enter the URL of your self-hosted Evey instance
      </Text>
      <TextInput
        placeholder="https://your-server.com"
        placeholderTextColor={theme.colors.textMuted}
        value={url}
        onChangeText={setUrl}
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        accessible={true}
        accessibilityLabel="Server URL"
        style={inputStyle}
      />
      <Pressable
        onPress={handleConnect}
        disabled={loading}
        accessible={true}
        accessibilityLabel="Connect to server"
        accessibilityRole="button"
        style={buttonStyle}
      >
        {loading ? (
          <ActivityIndicator color={theme.colors.text} />
        ) : (
          <Text style={{ color: "#fff", fontWeight: "600", fontSize: 16 }}>Connect</Text>
        )}
      </Pressable>
    </View>
  );
}

const scStyles = StyleSheet.create({
  heading: { fontSize: 24, fontWeight: "bold" },
});
