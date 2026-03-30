import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { setServerUrl } from "../src/lib/server-config";
import { initI18n } from "../src/i18n";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ServerConnect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleConnect() {
    const trimmed = url.trim().replace(/\/+$/, "");
    if (!trimmed) return;

    setLoading(true);
    try {
      const res = await fetch(`${trimmed}/healthz`);
      if (!res.ok) throw new Error(`Server returned ${res.status}`);
      await setServerUrl(trimmed);
      await initI18n(trimmed);
      router.replace("/login");
    } catch (e) {
      Alert.alert(
        t("mobile.serverConnect.connectionFailed"),
        e instanceof Error ? e.message : t("mobile.serverConnect.unreachable"),
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <View style={{ flex: 1, justifyContent: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.lg }}>
        {t("mobile.serverConnect.title")}
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
        style={{
          backgroundColor: theme.colors.surface,
          color: theme.colors.text,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          borderWidth: 1,
          borderColor: theme.colors.border,
          marginBottom: theme.spacing.md,
        }}
      />
      <Pressable
        onPress={handleConnect}
        disabled={loading}
        accessible={true}
        accessibilityLabel="Connect to server"
        accessibilityRole="button"
        style={{
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          alignItems: "center",
        }}
      >
        {loading ? (
          <ActivityIndicator color={theme.colors.text} />
        ) : (
          <Text style={{ color: "#fff", fontWeight: "600", fontSize: 16 }}>{t("mobile.serverConnect.connect")}</Text>
        )}
      </Pressable>
    </View>
  );
}
