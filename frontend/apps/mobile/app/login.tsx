import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";

export default function Login() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const setTokens = useAuthStore((s) => s.setTokens);

  const { mutate: login, isPending } = useUserServiceLogin({
    mutation: {
      onSuccess: (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          router.replace("/(auth)/(tabs)");
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

  const inputStyle = {
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
  };

  return (
    <View style={{ flex: 1, justifyContent: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.lg }}>
        {t("auth.login.signIn")}
      </Text>
      <TextInput
        placeholder={t("auth.login.username")}
        placeholderTextColor={theme.colors.textMuted}
        value={username}
        onChangeText={setUsername}
        autoCapitalize="none"
        autoCorrect={false}
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Username"
      />
      <TextInput
        placeholder={t("auth.login.password")}
        placeholderTextColor={theme.colors.textMuted}
        value={password}
        onChangeText={setPassword}
        secureTextEntry
        style={inputStyle}
        accessible={true}
        accessibilityLabel="Password"
      />
      <Pressable
        onPress={handleLogin}
        disabled={isPending}
        accessible={true}
        accessibilityLabel="Sign in"
        accessibilityRole="button"
        style={{
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.md,
          padding: theme.spacing.md,
          alignItems: "center",
        }}
      >
        {isPending ? (
          <ActivityIndicator color={theme.colors.text} />
        ) : (
          <Text style={{ color: "#fff", fontWeight: "600", fontSize: 16 }}>{t("auth.login.signIn")}</Text>
        )}
      </Pressable>
      <Pressable onPress={() => router.push("/forgot-password")} style={{ marginTop: theme.spacing.md }}>
        <Text style={{ color: theme.colors.accent, textAlign: "center" }}>{t("auth.login.forgotPassword")}</Text>
      </Pressable>
    </View>
  );
}
