import { useEffect, useState, useRef, useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceInitiateQuickConnect, userServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";
import { setRefreshToken } from "../src/lib/refresh-token";

const REFRESH_INTERVAL_S = 120;

export default function QuickConnectScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [code, setCode] = useState<string | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef(0);
  const setTokens = useAuthStore((s) => s.setTokens);
  const [backFocused, setBackFocused] = useState(false);

  const initiate = useUserServiceInitiateQuickConnect({
    mutation: {
      onSuccess: (data) => {
        if (data.code) {
          setCode(data.code);
          codeSetAt.current = Date.now();
          setSecondsLeft(REFRESH_INTERVAL_S);
        }
      },
    },
  });

  const doInitiate = useCallback(() => {
    initiate.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    doInitiate();
  }, [doInitiate]);

  useEffect(() => {
    const id = setInterval(() => {
      setCode(null);
      doInitiate();
    }, REFRESH_INTERVAL_S * 1000);
    return () => clearInterval(id);
  }, [doInitiate]);

  useEffect(() => {
    const id = setInterval(() => {
      const elapsed = Math.floor((Date.now() - codeSetAt.current) / 1000);
      setSecondsLeft(Math.max(0, REFRESH_INTERVAL_S - elapsed));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  // Poll for authorization. Web uses Server-Sent Events here, but
  // EventSource isn't a React Native global and Expo ships no SSE polyfill -
  // polling on the existing CheckQuickConnect REST endpoint is good enough
  // for a 2-minute code, same trade-off as mobile.
  const serverUrl = useServerUrlNullable();
  useEffect(() => {
    if (!code) return;
    let cancelled = false;

    async function tick() {
      if (cancelled || !code) return;
      try {
        const resp = await userServiceCheckQuickConnect(code);
        if (cancelled) return;
        if (resp.authorized && resp.auth?.accessToken) {
          if (resp.auth.refreshToken) await setRefreshToken(resp.auth.refreshToken);
          setTokens(resp.auth.accessToken);
          router.replace("/(auth)/(tabs)");
        }
      } catch {
        // Network blip - keep polling, the next tick retries.
      }
    }

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
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("auth.settings.quickConnect.title")}
      </Text>

      <Text style={[styles.instruction, { color: theme.colors.textMuted }]}>
        {t("auth.settings.quickConnect.scanInstruction")}
      </Text>

      <View style={[styles.codeBox, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
        {qrValue ? (
          <View style={{ backgroundColor: "#fff", padding: 16, borderRadius: 8, marginBottom: 24 }}>
            <QRCode value={qrValue} size={200} />
          </View>
        ) : (
          <ActivityIndicator color={theme.colors.accent} size="large" style={{ marginBottom: 24 }} />
        )}
        {displayCode ? (
          <Text style={[styles.code, { color: theme.colors.text }]}>
            {displayCode}
          </Text>
        ) : null}
      </View>

      {code && (
        <Text style={[styles.expires, { color: theme.colors.textMuted }]}>
          {t("auth.settings.quickConnect.expiresIn", { seconds: secondsLeft })}
        </Text>
      )}

      <Pressable
        onPress={() => router.back()}
        onFocus={() => setBackFocused(true)}
        onBlur={() => setBackFocused(false)}
        accessible={true}
        accessibilityLabel={t("auth.forgotPassword.backToLogin")}
        accessibilityRole="button"
        style={[
          styles.backButton,
          { borderColor: backFocused ? theme.colors.accent : "transparent" },
        ]}
      >
        <Text style={[styles.backText, { color: theme.colors.accent }]}>
          {t("common.back")}
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
    marginBottom: 16,
  },
  instruction: {
    fontSize: 20,
    textAlign: "center",
    maxWidth: 500,
    marginBottom: 48,
  },
  codeBox: {
    padding: 48,
    alignItems: "center",
    minWidth: 360,
  },
  code: {
    fontSize: 64,
    fontWeight: "bold",
    fontFamily: "monospace",
    letterSpacing: 16,
  },
  expires: {
    fontSize: 18,
    marginTop: 24,
  },
  backButton: {
    marginTop: 48,
    paddingVertical: 12,
    paddingHorizontal: 32,
    borderWidth: 3,
    borderRadius: 8,
  },
  backText: {
    fontSize: 22,
  },
});
