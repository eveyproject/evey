import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { setRefreshToken } from "../src/lib/refresh-token";

export default function Login() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [userFocused, setUserFocused] = useState(false);
  const [passFocused, setPassFocused] = useState(false);
  const [loginBtnFocused, setLoginBtnFocused] = useState(false);
  const [qcBtnFocused, setQcBtnFocused] = useState(false);
  const setTokens = useAuthStore((s) => s.setTokens);

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

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
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
    marginBottom: 40,
  },
  input: {
    width: 500,
    fontSize: 22,
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
    fontSize: 22,
  },
  divider: {
    width: 400,
    height: 1,
    marginVertical: 32,
  },
  qcButton: {
    width: 500,
    padding: 24,
    alignItems: "center",
    borderWidth: 3,
  },
  qcText: {
    fontWeight: "700",
    fontSize: 24,
  },
  qcHint: {
    fontSize: 16,
    marginTop: 4,
  },
});
