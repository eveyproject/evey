import { useState, useEffect, useMemo } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Alert, Linking, ScrollView } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import {
  useUserServiceLogin,
  useAdminSystemServiceGetSetupStatus,
  eveyFetch,
  formatWait,
  parseRateLimit,
  isRemoteAccessDisabled,
  remoteAccessDisabledMessage,
} from "@evey/api";
import { useAuthStore, useServerListStore, isDefaultServerName, toast } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrl } from "../src/hooks/useServerUrl";
import { setRefreshToken } from "../src/lib/refresh-token";
import { canSaveCredentials, getSavedCredentials, saveCredentials, clearSavedCredentials } from "../src/lib/saved-credentials";
import { useBiometricAvailable, useBiometricEnabled, authenticateBiometric } from "../src/hooks/useBiometric";
import { PasswordInput } from "../src/components/PasswordInput";
import { writeGuestSession } from "../src/features/syncplay/guestSession";

export default function Login() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(false);
  // Hidden until the keystore answers: a device without one can't remember a login.
  const [canRemember, setCanRemember] = useState(false);
  const [oidcLoading, setOidcLoading] = useState(false);
  const setTokens = useAuthStore((s) => s.setTokens);
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const { available: biometricAvailable, biometricType } = useBiometricAvailable();
  const { enabled: biometricEnabled } = useBiometricEnabled();
  const [hasSavedCreds, setHasSavedCreds] = useState(false);
  // "Watch together" guest join: no account needed, mirrors the web login.
  const [guestOpen, setGuestOpen] = useState(false);
  const [guestCode, setGuestCode] = useState("");
  const [guestJoining, setGuestJoining] = useState(false);
  const [guestError, setGuestError] = useState(false);
  // Biometric works two ways: with a live session it just unlocks; with saved
  // "remember me" credentials it performs a REAL login - landing here after a
  // session purge is one fingerprint away from being back in, not a retype.
  const canUseBiometric = biometricAvailable && biometricEnabled && (!!accessToken || hasSavedCreds);

  useEffect(() => {
    if (canUseBiometric) {
      handleBiometricLogin();
    }
  }, [canUseBiometric]);

  // Prefill from the keystore when the user opted into "remember me" - being
  // bounced to this screen (expired session, server hiccup) shouldn't mean
  // retyping credentials every time.
  useEffect(() => {
    let cancelled = false;
    void canSaveCredentials().then((ok) => {
      if (!cancelled) setCanRemember(ok);
    });
    void getSavedCredentials().then((saved) => {
      if (cancelled || !saved) return;
      setUsername((u) => u || saved.username);
      setPassword((p) => p || saved.password);
      setRemember(true);
      setHasSavedCreds(true);
    });
    return () => { cancelled = true; };
  }, []);

  async function handleBiometricLogin() {
    const success = await authenticateBiometric(t("mobile.auth.biometricPrompt") || "Unlock Evey");
    if (!success) return;
    if (accessToken) {
      router.replace("/profile-select");
      return;
    }
    const saved = await getSavedCredentials();
    if (saved) login({ data: { username: saved.username, password: saved.password } });
  }

  const oidcConfig = useQuery<{ enabled: boolean }>({
    queryKey: ["oidc-config"],
    queryFn: () => eveyFetch<{ enabled: boolean }>({ url: "/api/v1/auth/oidc/config", method: "GET" }),
    staleTime: 60_000,
  });

  // Offering "Forgot password" on a server without recovery configured sends
  // the user to a screen that can only apologize. Same public setup-status
  // gate the web login already uses.
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60_000 } });

  // The same answer names the instance: show it over the host, and keep the
  // saved-server row current so a rename made in the admin reaches the list.
  const advertisedName = setupStatus.data?.serverName?.trim();
  const activeServerId = useServerListStore((s) => s.activeServerId);
  const setServerName = useServerListStore((s) => s.setServerName);
  useEffect(() => {
    if (activeServerId && setupStatus.data) setServerName(activeServerId, advertisedName);
  }, [activeServerId, advertisedName, setupStatus.data, setServerName]);

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

  // t with a {{count}} param, for formatWait's unit strings.
  const tCount = (key: string, fallback: string, params?: Record<string, number>) =>
    t(key, fallback, params) as string;

  const { mutate: login, isPending } = useUserServiceLogin({
    mutation: {
      onSuccess: async (data) => {
        if (data.accessToken) {
          setTokens(data.accessToken);
          if (data.refreshToken) await setRefreshToken(data.refreshToken);
          if (remember && canRemember) {
            if (!(await saveCredentials(username.trim(), password.trim()))) {
              toast.error(t("mobile.auth.rememberFailed", "This device could not remember your login"));
            }
          } else await clearSavedCredentials();
          router.replace("/profile-select");
        }
      },
      onError: (err) => {
        // A rate-limited login is NOT a wrong password: telling the user to
        // check their credentials sends them retrying into the wall. Name the
        // block and the wait (2026-07-29).
        const status = (err as { httpStatus?: number })?.httpStatus;
        const limited = parseRateLimit(status ?? 0, null, err);
        if (limited) {
          Alert.alert(
            t("auth.rateLimit.title", "Too many attempts"),
            t("auth.rateLimit.body", "Too many login attempts. Try again in {{wait}}.")
              .replace("{{wait}}", formatWait(limited.retryAfterSeconds, tCount)),
          );
          return;
        }
        // A gated server refuses EVERY route to an off-LAN client - that is
        // not a wrong password, and telling the user to check credentials
        // sent one beta tester chasing a typo for two days (2026-07-29).
        if (isRemoteAccessDisabled(err)) {
          const { title, body } = remoteAccessDisabledMessage(t);
          Alert.alert(title, body);
          return;
        }
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

  // Join a "watch together" room as a guest (no account): the code mints a
  // media-scoped stream token; stash it and open the public guest player.
  async function handleGuestJoin() {
    const code = guestCode.trim();
    if (code.length < 4) return;
    setGuestJoining(true);
    setGuestError(false);
    try {
      const info = await eveyFetch<{ token: string; roomId: string; mediaId: string }>({
        url: "/api/v1/syncplay/join-code",
        method: "POST",
        data: { code },
      });
      if (!info.token || !info.mediaId || !info.roomId) throw new Error("invalid room");
      writeGuestSession({ token: info.token, roomId: info.roomId, mediaId: info.mediaId });
      router.push(`/watch/${info.mediaId}`);
    } catch {
      setGuestError(true);
    } finally {
      setGuestJoining(false);
    }
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
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ flexGrow: 1, justifyContent: "center", padding: theme.spacing.xl }}
    >
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: 4 }}>
        {t("auth.login.signIn")}
      </Text>
      {serverUrl && (
        <Pressable
          onPress={() => router.push("/server-connect")}
          style={{ marginBottom: theme.spacing.lg, flexDirection: "row", alignItems: "center", gap: 4 }}
          accessible={true}
          accessibilityLabel={t("mobile.settings.changeServer", "Change server")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
            {isDefaultServerName(advertisedName)
              ? serverUrl.replace(/^https?:\/\//, "")
              : `${advertisedName} - ${serverUrl.replace(/^https?:\/\//, "")}`}
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
        accessibilityLabel={t("auth.login.username")}
      />
      <PasswordInput
        placeholder={t("auth.login.password")}
        placeholderTextColor={theme.colors.textMuted}
        value={password}
        onChangeText={setPassword}
        style={inputStyle}
        accessible={true}
        accessibilityLabel={t("auth.login.password")}
      />
      {canRemember && (
      <Pressable
        onPress={() => setRemember((r) => !r)}
        accessible={true}
        accessibilityLabel={t("mobile.auth.rememberMe", "Remember me")}
        accessibilityRole="checkbox"
        accessibilityState={{ checked: remember }}
        style={{ flexDirection: "row", alignItems: "center", gap: 8, marginBottom: theme.spacing.md, minHeight: 44 }}
      >
        <Ionicons
          name={remember ? "checkbox" : "square-outline"}
          size={22}
          color={remember ? theme.colors.accent : theme.colors.textMuted}
        />
        <Text style={{ color: theme.colors.text, fontSize: 14 }}>
          {t("mobile.auth.rememberMe", "Remember me")}
        </Text>
      </Pressable>
      )}
      <Pressable
        onPress={handleLogin}
        disabled={isPending}
        accessible={true}
        accessibilityLabel={t("auth.login.signIn")}
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
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 }}>{t("auth.login.signIn")}</Text>
        )}
      </Pressable>
      {setupStatus.data?.passwordRecoveryEnabled && (
        <Pressable
          onPress={() => router.push("/forgot-password")}
          style={{ marginTop: theme.spacing.md }}
          accessible={true}
          accessibilityLabel={t("auth.login.forgotPassword")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.accent, textAlign: "center" }}>{t("auth.login.forgotPassword")}</Text>
        </Pressable>
      )}
      <Pressable
        onPress={() => router.push("/register")}
        style={{ marginTop: theme.spacing.sm }}
        accessible={true}
        accessibilityLabel={t("mobile.auth.haveInvite", "Have an invite? Sign up")}
        accessibilityRole="button"
      >
        <Text style={{ color: theme.colors.accent, textAlign: "center" }}>
          {t("mobile.auth.haveInvite", "Have an invite? Sign up")}
        </Text>
      </Pressable>

      {canUseBiometric && (
        <Pressable
          onPress={handleBiometricLogin}
          style={{ marginTop: theme.spacing.md, flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8 }}
          accessible={true}
          accessibilityLabel={t("mobile.auth.unlockBiometric") || "Unlock with biometrics"}
          accessibilityRole="button"
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
          accessibilityLabel={t("auth.settings.quickConnect.title")}
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
              accessibilityLabel={t("auth.login.signInWithSSO", "Sign in with SSO")}
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

        {/* Watch together - guest join by room code (no account needed). */}
        <View style={{ flexDirection: "row", alignItems: "center", marginVertical: theme.spacing.md, width: "100%" }}>
          <View style={{ flex: 1, height: 1, backgroundColor: theme.colors.border }} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginHorizontal: theme.spacing.sm }}>
            {t("auth.login.or", "or")}
          </Text>
          <View style={{ flex: 1, height: 1, backgroundColor: theme.colors.border }} />
        </View>
        {!guestOpen ? (
          <Pressable
            onPress={() => setGuestOpen(true)}
            accessibilityRole="button"
            accessibilityLabel={t("auth.login.guestJoinTrigger", "Watch together")}
            style={{ borderWidth: 1, borderColor: theme.colors.border, borderRadius: theme.radii.md, padding: theme.spacing.md, alignItems: "center", flexDirection: "row", justifyContent: "center", gap: 8, width: "100%" }}
          >
            <Ionicons name="people-outline" size={18} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 16 }}>
              {t("auth.login.guestJoinTrigger", "Watch together")}
            </Text>
          </Pressable>
        ) : (
          <View style={{ width: "100%", gap: theme.spacing.sm }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
              {t("auth.login.guestJoinDesc", "Enter the code - no account needed.")}
            </Text>
            <TextInput
              value={guestCode}
              onChangeText={(v) => { setGuestCode(v); setGuestError(false); }}
              placeholder="ABC123"
              placeholderTextColor={theme.colors.textMuted}
              autoCapitalize="characters"
              autoCorrect={false}
              style={inputStyle}
              accessibilityLabel={t("media.syncplay.codeLabel", "Code")}
            />
            <Pressable
              onPress={() => void handleGuestJoin()}
              disabled={guestJoining || guestCode.trim().length < 4}
              accessibilityRole="button"
              accessibilityLabel={t("media.syncplay.joinRoom", "Join")}
              style={{ borderWidth: 1, borderColor: theme.colors.border, borderRadius: theme.radii.md, padding: theme.spacing.md, alignItems: "center", width: "100%", opacity: guestJoining || guestCode.trim().length < 4 ? 0.5 : 1 }}
            >
              {guestJoining ? (
                <ActivityIndicator color={theme.colors.text} size="small" />
              ) : (
                <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 16 }}>
                  {t("media.syncplay.joinRoom", "Join")}
                </Text>
              )}
            </Pressable>
            {guestError && (
              <Text style={{ color: theme.colors.error, fontSize: 13 }}>
                {t("auth.login.guestJoinError", "Could not join - check the code.")}
              </Text>
            )}
          </View>
        )}
      </View>
    </ScrollView>
  );
}
