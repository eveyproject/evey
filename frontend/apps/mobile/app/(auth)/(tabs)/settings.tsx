import { useEffect, useState } from "react";
import { View, Text, ScrollView, Pressable, Alert, StyleSheet } from "react-native";
import { router } from "expo-router";
import Constants from "expo-constants";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useAdminServiceGetSystemInfo } from "@evey/api";
import { useAuthStore, useThemeStore, useLocaleStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { getServerUrl, clearServerUrl } from "../../../src/lib/server-config";
import { supportedLanguages } from "../../../src/i18n";

const THEME_OPTIONS = [
  { label: "Dark", value: "default" },
  { label: "Light", value: "light-default" },
  { label: "AMOLED", value: "amoled" },
] as const;

function ChevronRight({ color }: { color: string }) {
  return <Text style={{ color, fontSize: 18 }}>{"›"}</Text>;
}

export default function SettingsScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const logout = useAuthStore((s) => s.logout);
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const [serverUrl, setServerUrl] = useState<string | null>(null);

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: systemInfo } = useAdminServiceGetSystemInfo();

  useEffect(() => {
    getServerUrl().then(setServerUrl);
  }, []);

  async function handleChangeServer() {
    Alert.alert(t("mobile.settings.changeServer"), t("mobile.settings.disconnectConfirm"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("mobile.settings.disconnect"),
        style: "destructive",
        onPress: async () => {
          logout();
          await clearServerUrl();
          router.replace("/server-connect");
        },
      },
    ]);
  }

  async function handleSignOut() {
    Alert.alert(t("mobile.settings.signOut"), t("mobile.settings.signOutConfirm"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("mobile.settings.signOut"),
        style: "destructive",
        onPress: () => {
          logout();
          router.replace("/login");
        },
      },
    ]);
  }

  function handleLanguageChange(code: string) {
    setLocale(code);
    i18n.changeLanguage(code);
  }

  const appVersion = Constants.expoConfig?.version ?? "1.0.0";
  const currentLang = i18n.language;

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>
      {/* Server */}
      <Text style={s.sectionTitle}>{t("mobile.settings.server")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>URL</Text>
          <Text style={s.value} numberOfLines={1}>{serverUrl ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={handleChangeServer}>
          <Text style={s.labelDestructive}>{t("mobile.settings.changeServer")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Account */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.account")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>{t("auth.login.username")}</Text>
          <Text style={s.value}>{user?.username ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={handleSignOut}>
          <Text style={s.labelDestructive}>{t("mobile.settings.signOut")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Language */}
      <Text style={s.sectionTitle}>{t("common.language")}</Text>
      <View style={s.section}>
        {supportedLanguages.map((lang) => (
          <Pressable key={lang.code} style={s.row} onPress={() => handleLanguageChange(lang.code)}>
            <Text style={s.label}>{lang.label}</Text>
            {currentLang === lang.code && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>✓</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* Appearance */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.appearance")}</Text>
      <View style={s.section}>
        {THEME_OPTIONS.map((opt) => (
          <Pressable key={opt.value} style={s.row} onPress={() => setTheme(opt.value)}>
            <Text style={s.label}>{opt.label}</Text>
            {currentTheme === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>✓</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* About */}
      <Text style={s.sectionTitle}>{t("mobile.settings.about")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>{t("mobile.settings.appVersion")}</Text>
          <Text style={s.value}>{appVersion}</Text>
        </View>
        <View style={s.row}>
          <Text style={s.label}>{t("mobile.settings.serverVersion")}</Text>
          <Text style={s.value}>{systemInfo?.version ?? "—"}</Text>
        </View>
      </View>
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: {
      flex: 1,
      backgroundColor: t.colors.bg,
    },
    sectionTitle: {
      color: t.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: t.spacing.lg,
      marginBottom: t.spacing.xs,
      marginHorizontal: t.spacing.md,
    },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      overflow: "hidden",
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      paddingHorizontal: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
    label: {
      color: t.colors.text,
      fontSize: 16,
    },
    labelDestructive: {
      color: "#ef4444",
      fontSize: 16,
    },
    value: {
      color: t.colors.textMuted,
      fontSize: 15,
      flexShrink: 1,
      marginLeft: t.spacing.md,
    },
  });
