import { useEffect, useState, useRef, useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, StyleSheet } from "react-native";
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

  // Everything has to FIT the panel: the only focusable element is the Back
  // button at the bottom, so on a television the ScrollView scrolls to it and
  // whatever does not fit is lost at the top - the title was cut in half on
  // the owner's set at 80% density, where the panel is 675dp and this page
  // added up to about 720 (2026-09-20). Budgeted below for ~560dp; the
  // ScrollView only remains for panels smaller than that.
  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={styles.container}
    >
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("auth.settings.quickConnect.title")}
      </Text>

      <Text style={[styles.instruction, { color: theme.colors.textMuted }]}>
        {t("auth.settings.quickConnect.scanInstruction")}
      </Text>

      <View style={[styles.codeBox, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
        {qrValue ? (
          <View style={{ backgroundColor: "#fff", padding: 12, borderRadius: 8, marginBottom: 12 }}>
            <QRCode value={qrValue} size={170} />
          </View>
        ) : initiate.isError ? (
          // The code request runs on mount: when it fails there is no gesture to
          // repeat, and a remote cannot reload the screen - so the spinner spun
          // for ever with nothing to press.
          <Pressable
            onPress={doInitiate}
            testID="quick-connect-retry"
            accessibilityLabel={t("common.retry")}
            style={{ marginBottom: 24, paddingVertical: 9, paddingHorizontal: 28, backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }}
          >
            <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "600" }}>{t("common.retry")}</Text>
          </Pressable>
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
        // The only focusable on this screen, and there is no sidebar behind it:
        // without an initial claim Android has no focused view and stops
        // delivering key events, so the viewer cannot even leave the QR code.
        hasTVPreferredFocus
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
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {
    flexGrow: 1,
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: 80,
    paddingTop: 24,
    paddingBottom: 32,
  },
  title: {
    fontSize: 38,
    fontWeight: "bold",
    marginBottom: 8,
  },
  instruction: {
    fontSize: 18,
    textAlign: "center",
    maxWidth: 500,
    marginBottom: 16,
  },
  codeBox: {
    padding: 24,
    alignItems: "center",
    minWidth: 360,
  },
  code: {
    fontSize: 52,
    fontWeight: "bold",
    fontFamily: "monospace",
    letterSpacing: 14,
  },
  expires: {
    fontSize: 17,
    marginTop: 12,
  },
  backButton: {
    marginTop: 16,
    paddingVertical: 9,
    paddingHorizontal: 32,
    borderWidth: 3,
    borderRadius: 8,
  },
  backText: {
    fontSize: 20,
  },
});
