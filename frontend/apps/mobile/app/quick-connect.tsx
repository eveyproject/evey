import { useEffect, useState, useRef, useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator } from "react-native";
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
  const serverUrl = useServerUrlNullable();
  const [code, setCode] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef(0);
  const setTokens = useAuthStore((s) => s.setTokens);

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

  // On auth success
  useEffect(() => {
    if (checkData?.authorized && checkData.auth?.accessToken) {
      setEnabled(false);
      setTokens(checkData.auth.accessToken);
      router.replace("/(auth)/(tabs)");
    }
  }, [checkData, setTokens]);

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
        onPress={() => router.back()}
        style={{ marginTop: theme.spacing.xl }}
        accessible={true}
        accessibilityLabel="Back to login"
        accessibilityRole="button"
      >
        <Text style={{ color: theme.colors.accent, fontSize: 16 }}>{t("common.back")}</Text>
      </Pressable>
    </View>
  );
}
