import { useState, useEffect } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { setServerUrl, getServerUrl } from "../src/lib/server-config";
import { initI18n } from "../src/i18n";
import { useTheme } from "../src/providers/ThemeProvider";

export default function ServerConnect() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    getServerUrl().then((saved) => { if (saved) setUrl(saved); });
  }, []);
  const [inputFocused, setInputFocused] = useState(false);
  const [buttonFocused, setButtonFocused] = useState(false);

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
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("mobile.serverConnect.title")}
      </Text>
      <Text style={[styles.subtitle, { color: theme.colors.textMuted }]}>
        Enter your Evey server address
      </Text>
      <TextInput
        placeholder="https://your-server.com"
        placeholderTextColor={theme.colors.textMuted}
        value={url}
        onChangeText={setUrl}
        autoCapitalize="none"
        autoCorrect={false}
        onFocus={() => setInputFocused(true)}
        onBlur={() => setInputFocused(false)}
        onSubmitEditing={handleConnect}
        accessible={true}
        accessibilityLabel="Server URL"
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            borderColor: inputFocused ? theme.colors.accent : theme.colors.border,
          },
        ]}
      />
      <Pressable
        onPress={handleConnect}
        disabled={loading}
        onFocus={() => setButtonFocused(true)}
        onBlur={() => setButtonFocused(false)}
        accessible={true}
        accessibilityLabel="Connect to server"
        accessibilityRole="button"
        style={[
          styles.button,
          {
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.md,
            borderColor: buttonFocused ? "#fff" : "transparent",
          },
        ]}
      >
        {loading ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={styles.buttonText}>{t("mobile.serverConnect.connect")}</Text>
        )}
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 80,
  },
  title: {
    fontSize: 42,
    fontWeight: "bold",
    marginBottom: 12,
  },
  subtitle: {
    fontSize: 20,
    marginBottom: 40,
  },
  input: {
    width: 500,
    fontSize: 22,
    padding: 18,
    borderWidth: 3,
    marginBottom: 24,
  },
  button: {
    width: 500,
    padding: 18,
    alignItems: "center",
    borderWidth: 3,
  },
  buttonText: {
    color: "#fff",
    fontWeight: "700",
    fontSize: 22,
  },
});
