import { useCallback, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, BackHandler, ScrollView, StyleSheet } from "react-native";
import { router, useFocusEffect } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { setRefreshToken } from "../src/lib/refresh-token";
import { clearServerUrl } from "../src/lib/server-config";
import { queryClient } from "../src/lib/query-client";

export default function Login() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [userFocused, setUserFocused] = useState(false);
  const [passFocused, setPassFocused] = useState(false);
  const [loginBtnFocused, setLoginBtnFocused] = useState(false);
  const [qcBtnFocused, setQcBtnFocused] = useState(false);
  const [serverBtnFocused, setServerBtnFocused] = useState(false);
  const setTokens = useAuthStore((s) => s.setTokens);

  // Back to the server screen: this is the root once a server is chosen, so
  // the remote's BACK key otherwise leaves the app for the Android home - a
  // viewer who picked the wrong server (the LAN scan lists every Evey it
  // finds, a dev box included) had no way back. The stored URL is cleared
  // so the scan runs again instead of auto-connecting to the same server.
  const changeServer = useCallback(async () => {
    queryClient.clear();
    await clearServerUrl();
    router.replace("/server-connect");
  }, []);
  // Only while THIS screen is focused: Quick Connect is pushed on top of it,
  // and BACK there must pop back here, not jump to the server screen.
  useFocusEffect(useCallback(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      void changeServer();
      return true;
    });
    return () => sub.remove();
  }, [changeServer]));

  const { mutate: login, isPending } = useUserServiceLogin({
    mutation: {
      onSuccess: async (data) => {
        if (data.accessToken) {
          // Without the refresh token the session dies with the access token
          // and the TV asks for a password again on the next cold start.
          if (data.refreshToken) await setRefreshToken(data.refreshToken);
          setTokens(data.accessToken);
          router.replace("/profile-select");
        }
      },
      onError: (err) => {
        Alert.alert(
          t("auth.login.loginFailed"),
          err instanceof Error ? err.message : t("auth.login.invalidCredentials"),
        );
      },
    },
  });

  function handleLogin() {
    if (!username.trim() || !password) return;
    login({ data: { username: username.trim(), password } });
  }

  // A ScrollView, not a centred View: with the soft keyboard up the window
  // shrinks to the top half of the screen and the sign-in and Quick Connect
  // buttons were simply out of reach (prod, 2026-09-19).
  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={styles.container}
      keyboardShouldPersistTaps="handled"
    >
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("auth.login.signIn")}
      </Text>

      <TextInput
        placeholder={t("auth.login.username")}
        placeholderTextColor={theme.colors.textMuted}
        value={username}
        onChangeText={setUsername}
        autoCapitalize="none"
        autoCorrect={false}
        onFocus={() => setUserFocused(true)}
        onBlur={() => setUserFocused(false)}
        accessible={true}
        accessibilityLabel={t("auth.login.username")}
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            borderColor: userFocused ? theme.colors.accent : theme.colors.border,
          },
        ]}
      />
      <TextInput
        placeholder={t("auth.login.password")}
        placeholderTextColor={theme.colors.textMuted}
        value={password}
        onChangeText={setPassword}
        secureTextEntry
        onFocus={() => setPassFocused(true)}
        onBlur={() => setPassFocused(false)}
        onSubmitEditing={handleLogin}
        accessible={true}
        accessibilityLabel={t("auth.login.password")}
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            borderColor: passFocused ? theme.colors.accent : theme.colors.border,
          },
        ]}
      />

      <Pressable
        onPress={handleLogin}
        disabled={isPending}
        onFocus={() => setLoginBtnFocused(true)}
        onBlur={() => setLoginBtnFocused(false)}
        accessible={true}
        accessibilityLabel={t("auth.login.signIn")}
        accessibilityRole="button"
        style={[
          styles.button,
          {
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.md,
            borderColor: loginBtnFocused ? "#fff" : "transparent",
          },
        ]}
      >
        {isPending ? (
          <ActivityIndicator color={theme.colors.onAccent} />
        ) : (
          <Text style={[styles.buttonText, { color: theme.colors.onAccent }]}>{t("auth.login.signIn")}</Text>
        )}
      </Pressable>

      <View style={[styles.divider, { backgroundColor: theme.colors.border }]} />

      <Pressable
        onPress={() => router.push("/quick-connect")}
        onFocus={() => setQcBtnFocused(true)}
        onBlur={() => setQcBtnFocused(false)}
        hasTVPreferredFocus={true}
        accessible={true}
        accessibilityLabel={t("auth.settings.quickConnect.title")}
        accessibilityRole="button"
        style={[
          styles.qcButton,
          {
            borderColor: qcBtnFocused ? theme.colors.accent : theme.colors.border,
            borderRadius: theme.radii.md,
          },
        ]}
      >
        <Text style={[styles.qcText, { color: theme.colors.text }]}>
          {t("auth.settings.quickConnect.title")}
        </Text>
        <Text style={[styles.qcHint, { color: theme.colors.textMuted }]}>
          {t("auth.login.recommendedForTV")}
        </Text>
      </Pressable>

      <Pressable
        onPress={() => { void changeServer(); }}
        onFocus={() => setServerBtnFocused(true)}
        onBlur={() => setServerBtnFocused(false)}
        accessible={true}
        accessibilityLabel={t("mobile.settings.changeServer")}
        accessibilityRole="button"
        style={[styles.serverButton, { borderColor: serverBtnFocused ? theme.colors.accent : "transparent", borderRadius: theme.radii.md }]}
      >
        <Text style={[styles.serverText, { color: serverBtnFocused ? theme.colors.text : theme.colors.textMuted }]}>
          {t("mobile.settings.changeServer")}
        </Text>
      </Pressable>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {
    flexGrow: 1,
    justifyContent: "center",
    alignItems: "center",
    // A television overscans: the last row must not sit on the very edge of
    // the panel, and the content is centred with room to scroll when the
    // soft keyboard takes the bottom half.
    paddingHorizontal: 80,
    paddingTop: 48,
    paddingBottom: 72,
  },
  title: {
    fontSize: 42,
    fontWeight: "bold",
    marginBottom: 32,
  },
  input: {
    width: 500,
    fontSize: 20,
    padding: 18,
    borderWidth: 3,
    marginBottom: 20,
  },
  button: {
    width: 500,
    padding: 18,
    alignItems: "center",
    borderWidth: 3,
    marginTop: 8,
  },
  buttonText: {
    fontWeight: "700",
    fontSize: 20,
  },
  divider: {
    width: 400,
    height: 1,
    marginVertical: 24,
  },
  qcButton: {
    width: 500,
    padding: 24,
    alignItems: "center",
    borderWidth: 3,
  },
  qcText: {
    fontWeight: "700",
    fontSize: 21,
  },
  qcHint: {
    fontSize: 15,
    marginTop: 4,
  },
  serverButton: {
    marginTop: 16,
    paddingVertical: 9,
    paddingHorizontal: 24,
    borderWidth: 3,
  },
  serverText: {
    fontSize: 18,
  },
});
