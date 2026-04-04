import { useState, useEffect, useMemo } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, Linking } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { useUserServiceLogin, eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrl } from "../src/hooks/useServerUrl";
import { setRefreshToken } from "../src/lib/refresh-token";
import { useBiometricAvailable, useBiometricEnabled, authenticateBiometric } from "../src/hooks/useBiometric";

export default function Login() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [oidcLoading, setOidcLoading] = useState(false);
  const setTokens = useAuthStore((s) => s.setTokens);
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const { available: biometricAvailable, biometricType } = useBiometricAvailable();
  const { enabled: biometricEnabled } = useBiometricEnabled();
  const canUseBiometric = biometricAvailable && biometricEnabled && !!accessToken;

  useEffect(() => {
    if (canUseBiometric) {
      handleBiometricLogin();
    }
  }, [canUseBiometric]);

  async function handleBiometricLogin() {
    const success = await authenticateBiometric(t("mobile.auth.biometricPrompt") || "Unlock Evey");
    if (success) {
      router.replace("/profile-select");
    }
  }

  const oidcConfig = useQuery<{ enabled: boolean }>({
    queryKey: ["oidc-config"],
    queryFn: () => eveyFetch<{ enabled: boolean }>({ url: "/api/v1/auth/oidc/config", method: "GET" }),
    staleTime: 60_000,
  });

  // Handle OIDC deep link callback (evey://oidc-callback)
  useEffect(() => {
    function handleUrl(event: { url: string }) {
      if (!event.url.includes("oidc-callback")) return;
      const url = new URL(event.url);
      const oidcError = url.searchParams.get("oidc_error");
      if (oidcError) {
        Alert.alert(t("auth.login.ssoFailed", "SSO login failed"));
        return;
      }
      (async () => {
        try {
          const data = await eveyFetch<{ accessToken: string; refreshToken?: string }>({
            url: "/api/v1/auth/oidc/session",
            method: "GET",
          });
          setTokens(data.accessToken);
          if (data.refreshToken) await setRefreshToken(data.refreshToken);
          router.replace("/profile-select");
        } catch {
          Alert.alert(t("auth.login.ssoFailed", "SSO login failed"));
        }
      })();
    }
    const sub = Linking.addEventListener("url", handleUrl);
    return () => sub.remove();
  }, [setTokens, t]);

  async function handleOidcLogin() {
    if (!serverUrl) return;
    setOidcLoading(true);
    try {
      await Linking.openURL(`${serverUrl}/api/v1/auth/oidc/login?redirect_uri=${encodeURIComponent("evey://oidc-callback")}`);
    } catch {
      Alert.alert(t("auth.login.ssoFailed", "Could not open SSO login"));
    } finally {
      setOidcLoading(false);
    }
  }

  const { mutate: login, isPending } = useUserServiceLogin({
    mutation: {
      onSuccess: async (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          if (data.refreshToken) await setRefreshToken(data.refreshToken);
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
    const u = username.trim();
    const p = password.trim();
    if (!u || !p) return;
    login({ data: { username: u, password: p } });
  }

  const inputStyle = useMemo(() => ({
    backgroundColor: theme.colors.surface,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    marginBottom: theme.spacing.md,
  }), [theme]);

  return (
    <View style={{ flex: 1, justifyContent: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: 4 }}>
        {t("auth.login.signIn")}
      </Text>
      {serverUrl && (
        <Pressable onPress={() => router.push("/server-connect")} style={{ marginBottom: theme.spacing.lg, flexDirection: "row", alignItems: "center", gap: 4 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {serverUrl.replace(/^https?:\/\//, "")}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 12 }}>{t("common.change", "Change")}</Text>
        </Pressable>
      )}
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
      <Pressable onPress={() => router.push("/register")} style={{ marginTop: theme.spacing.sm }}>
        <Text style={{ color: theme.colors.accent, textAlign: "center" }}>
          {t("mobile.auth.haveInvite", "Have an invite? Sign up")}
        </Text>
      </Pressable>

      {canUseBiometric && (
        <Pressable
          onPress={handleBiometricLogin}
          style={{ marginTop: theme.spacing.md, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8 }}
        >
          <Ionicons name={biometricType === "face" ? "scan-outline" : "finger-print-outline"} size={20} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.accent, fontSize: 14, fontWeight: "600" }}>
            {t("mobile.auth.unlockBiometric") || "Unlock with biometrics"}
          </Text>
        </Pressable>
      )}

      <View style={{ marginTop: theme.spacing.xl, alignItems: "center" }}>
        <View style={{ width: "80%", height: 1, backgroundColor: theme.colors.border, marginBottom: theme.spacing.lg }} />
        <Pressable
          onPress={() => router.push("/quick-connect")}
          accessible={true}
          accessibilityLabel="Quick Connect"
          accessibilityRole="button"
          style={{
            borderWidth: 1,
            borderColor: theme.colors.border,
            borderRadius: theme.radii.md,
            padding: theme.spacing.md,
            alignItems: "center",
            width: "100%",
          }}
        >
          <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 16 }}>
            {t("auth.settings.quickConnect.title")}
          </Text>
        </Pressable>

        {oidcConfig.data?.enabled && (
          <>
            <View style={{ flexDirection: "row", alignItems: "center", marginVertical: theme.spacing.md, width: "100%" }}>
              <View style={{ flex: 1, height: 1, backgroundColor: theme.colors.border }} />
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginHorizontal: theme.spacing.sm }}>
                {t("auth.login.or", "or")}
              </Text>
              <View style={{ flex: 1, height: 1, backgroundColor: theme.colors.border }} />
            </View>
            <Pressable
              onPress={() => void handleOidcLogin()}
              disabled={oidcLoading}
              accessible={true}
              accessibilityLabel="Sign in with SSO"
              accessibilityRole="button"
              style={{
                borderWidth: 1,
                borderColor: theme.colors.border,
                borderRadius: theme.radii.md,
                padding: theme.spacing.md,
                alignItems: "center",
                width: "100%",
              }}
            >
              {oidcLoading ? (
                <ActivityIndicator color={theme.colors.text} size="small" />
              ) : (
                <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 16 }}>
                  {t("auth.login.signInWithSSO", "Sign in with SSO")}
                </Text>
              )}
            </Pressable>
          </>
        )}
      </View>
    </View>
  );
}
