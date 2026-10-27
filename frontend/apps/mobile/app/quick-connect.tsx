import { useEffect, useState, useRef, useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceInitiateQuickConnect, checkQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { setRefreshToken } from "../src/lib/refresh-token";
import { LoadFailed } from "../src/components/LoadFailed";

const REFRESH_INTERVAL_S = 120;

export default function QuickConnectScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrlNullable();
  const [code, setCode] = useState<string | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef(0);
  // The secret comes back with the code and never leaves this device: the
  // QR carries the code, the poll carries it as a header, so a phone that scanned the
  // screen cannot collect the tokens in our place.
  const secretRef = useRef<string | null>(null);
  const setTokens = useAuthStore((s) => s.setTokens);

  const initiate = useUserServiceInitiateQuickConnect({
    mutation: {
      onSuccess: (data) => {
        if (data.code) {
          setCode(data.code);
          secretRef.current = data.secret ?? null;
          codeSetAt.current = Date.now();
          setSecondsLeft(REFRESH_INTERVAL_S);
        }
      },
      onError: () => setCode(null),
    },
  });

  const doInitiate = useCallback(() => {
    initiate.mutate({ params: { secretAware: true } });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    doInitiate();
  }, [doInitiate]);

  // Auto-refresh code
  useEffect(() => {
    const id = setInterval(() => {
      setCode(null);
      doInitiate();
    }, REFRESH_INTERVAL_S * 1000);
    return () => clearInterval(id);
  }, [doInitiate]);

  // Countdown
  useEffect(() => {
    const id = setInterval(() => {
      const elapsed = Math.floor((Date.now() - codeSetAt.current) / 1000);
      setSecondsLeft(Math.max(0, REFRESH_INTERVAL_S - elapsed));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  // Poll for authorization. Web uses Server-Sent Events here, but
  // EventSource isn't a React Native global and Expo Go ships no SSE
  // polyfill - polling on the existing CheckQuickConnect REST endpoint
  // is good enough for a 2-minute code (avg ~1.5s extra latency over SSE).
  useEffect(() => {
    if (!code) return;
    let cancelled = false;
    // One check at a time: a slow answer must not let the next tick fire a
    // second one, which could sign in twice or race the navigation.
    let inFlight = false;

    async function tick() {
      if (cancelled || !code || inFlight) return;
      inFlight = true;
      try {
        const resp = await checkQuickConnect(code, secretRef.current);
        if (cancelled) return;
        if (resp.authorized && resp.auth?.accessToken) {
          setTokens(resp.auth.accessToken);
          // The server issues a refresh token here exactly as it does for a
          // password login (CheckQuickConnect calls GenerateTokensForUser).
          // Dropping it made this the only sign-in path that produced a
          // 15-minute session: at the access token's expiry the refresher
          // found nothing in SecureStore, read the absence as a dead session,
          // and logged the user back out minutes after they signed in.
          if (resp.auth.refreshToken) await setRefreshToken(resp.auth.refreshToken);
          router.replace("/(auth)/(tabs)");
        }
      } catch {
        // Network blip - keep polling, the next tick retries.
      } finally {
        inFlight = false;
      }
    }

    // 3s poll - short enough for snappy
    // authorize→tab handoff, long enough not to hammer the server when 50
    // devices are sitting on the screen. Web uses SSE so it's instant; we
    // can't on mobile.
    const id = setInterval(tick, 3000);
    void tick(); // first poll immediately so authorize-then-tab is snappy
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [code, setTokens]);

  const displayCode = code ? code.slice(-6).toUpperCase() : null;
  const qrValue = code && serverUrl ? `${serverUrl}/quickconnect?code=${code}` : null;

  return (
    <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl, backgroundColor: theme.colors.bg }}>
      <Text style={{ fontSize: 24, fontWeight: "bold", color: theme.colors.text, marginBottom: theme.spacing.lg }}>
        {t("auth.settings.quickConnect.title")}
      </Text>

      <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center", marginBottom: theme.spacing.lg, maxWidth: 280 }}>
        {t("auth.settings.quickConnect.scanInstruction")}
      </Text>

      <View style={{
        backgroundColor: theme.colors.surface,
        borderRadius: theme.radii.md,
        padding: theme.spacing.xl,
        alignItems: "center",
        minWidth: 200,
        gap: theme.spacing.md,
      }}>
        {qrValue ? (
          <View style={{ backgroundColor: "#fff", padding: 12, borderRadius: 8 }}>
            <QRCode value={qrValue} size={160} />
          </View>
        ) : initiate.isError ? (
          <LoadFailed inline onRetry={doInitiate} />
        ) : (
          <ActivityIndicator color={theme.colors.accent} size="large" />
        )}
        {displayCode ? (
          <Text style={{ fontSize: 32, fontWeight: "bold", fontFamily: "monospace", letterSpacing: 8, color: theme.colors.text }}>
            {displayCode}
          </Text>
        ) : null}
      </View>

      {code && (
        <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.md }}>
          {t("auth.settings.quickConnect.expiresIn", { seconds: secondsLeft })}
        </Text>
      )}

      <Pressable
        onPress={() => router.push("/(auth)/qr-scan")}
        style={{
          marginTop: theme.spacing.lg,
          backgroundColor: theme.colors.accent,
          borderRadius: theme.radii.md,
          paddingVertical: 12,
          paddingHorizontal: 24,
          flexDirection: "row",
          alignItems: "center",
          gap: 8,
        }}
        accessible={true}
        accessibilityLabel={t("auth.settings.quickConnect.scanQr")}
        accessibilityRole="button"
      >
        <Text style={{ fontSize: 16 }}>📷</Text>
        <Text style={{ color: theme.colors.onAccent, fontSize: 15, fontWeight: "600" }}>{t("auth.settings.quickConnect.scanQr") || "Scan QR Code"}</Text>
      </Pressable>

      <Pressable
        onPress={() => router.back()}
        style={{ marginTop: theme.spacing.md }}
        accessible={true}
        accessibilityLabel={t("auth.forgotPassword.backToLogin")}
        accessibilityRole="button"
      >
        <Text style={{ color: theme.colors.accent, fontSize: 16 }}>{t("common.back")}</Text>
      </Pressable>
    </View>
  );
}
