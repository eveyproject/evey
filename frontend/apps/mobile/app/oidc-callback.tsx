import { useEffect, useRef } from "react";
import { View, ActivityIndicator, Alert } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { ssoErrorKey } from "@evey/client";
import { useTheme } from "../src/providers/ThemeProvider";
import { setRefreshToken } from "../src/lib/refresh-token";
import { takeSsoPollToken } from "../src/lib/ssoFlow";

// Target of evey://oidc-callback, where the server sends the browser after an
// SSO sign-in started from the login screen. It redeems the one-time code with
// the poll secret kept in memory since the start, then leaves.
export default function OidcCallbackScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const setTokens = useAuthStore((s) => s.setTokens);
  const params = useLocalSearchParams<{ code?: string; oidc_error?: string }>();
  const handled = useRef(false);

  useEffect(() => {
    if (handled.current) return;
    handled.current = true;
    const pollToken = takeSsoPollToken();
    const fail = (key: string) => {
      Alert.alert(t(key));
      router.replace("/login");
    };
    if (params.oidc_error || !params.code || !pollToken) {
      fail(ssoErrorKey(params.oidc_error));
      return;
    }
    (async () => {
      try {
        const data = await eveyFetch<{ accessToken: string; refreshToken?: string }>({
          url: "/api/v1/auth/oidc/app/token",
          method: "POST",
          data: { code: params.code, pollToken },
        });
        setTokens(data.accessToken);
        if (data.refreshToken) await setRefreshToken(data.refreshToken);
        router.replace("/profile-select");
      } catch {
        fail("auth.login.ssoFailed");
      }
    })();
  }, [params.code, params.oidc_error, setTokens, t]);

  return (
    <View style={{ flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: theme.colors.bg }}>
      <ActivityIndicator color={theme.colors.text} />
    </View>
  );
}
