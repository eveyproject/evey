import { useEffect, useState, useRef, useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import QRCode from "react-native-qrcode-svg";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceInitiateQuickConnect, useUserServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../src/hooks/useServerUrl";

const REFRESH_INTERVAL_S = 120;

export default function QuickConnectScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [code, setCode] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
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

  const { data: checkData } = useUserServiceCheckQuickConnect(code ?? "", {
    query: {
      enabled: !!code && enabled,
      refetchInterval: 3000,
      refetchIntervalInBackground: true,
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

  useEffect(() => {
    if (checkData?.authorized && checkData.auth?.accessToken) {
      setEnabled(false);
      setTokens(checkData.auth.accessToken);
      router.replace("/(auth)/(tabs)");
    }
  }, [checkData, setTokens]);

  const serverUrl = useServerUrlNullable();
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
        accessibilityLabel="Back to login"
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
