import { useState, useCallback } from "react";
import { View, Text, ScrollView, Pressable, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, useAdminSystemServiceGetSystemInfo, userServiceLogout, eveyFetch } from "@evey/api";
import { useAuthStore, useThemeStore, useLocaleStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../../../src/hooks/useServerUrl";
import { clearServerUrl } from "../../../src/lib/server-config";
import { supportedLanguages } from "../../../src/i18n";

// labelKey/fallback pairs mirror mobile's ThemeSelector.tsx builtInI18nKeys
// mapping - same three built-in themes this TV settings screen exposes.
const THEME_OPTIONS = [
  { labelKey: "mobile.settings.themeDark", fallback: "Dark", value: "default" },
  { labelKey: "mobile.settings.themeLight", fallback: "Light", value: "light-default" },
  { labelKey: "mobile.settings.themeAmoled", fallback: "AMOLED", value: "amoled" },
] as const;

export default function SettingsScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const logout = useAuthStore((s) => s.logout);
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const serverUrl = useServerUrlNullable();

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p: any) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile();
  const { data: systemInfo } = useAdminSystemServiceGetSystemInfo();

  const [focusedItem, setFocusedItem] = useState<string | null>(null);
  const currentLang = i18n.language;

  async function handleLanguageChange(code: string) {
    setLocale(code);
    void i18n.changeLanguage(code);
    if (defaultProfile?.id) {
      try {
        await updateProfile.mutateAsync({ id: defaultProfile.id, data: { locale: code } });
        const resp = await eveyFetch<{ accessToken?: string }>({ url: "/api/v1/auth/refresh", method: "POST", data: {} });
        if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      } catch {
        // The locale was already applied in-app; the server-side persist
        // failure is best-effort and surfaces on next refresh.
      }
    }
  }

  async function handleChangeServer() {
    logout();
    await clearServerUrl();
    router.replace("/server-connect");
  }

  function handleSignOut() {
    // Clear local state and navigate FIRST: on a shared/offline TV, sign-out
    // must never appear to hang, or worse, silently stay logged in, waiting
    // on a network call. The server-side revoke is best-effort and its
    // failure must never be allowed to leave the refresh/stream tokens alive
    // while the UI already shows the login screen.
    logout();
    router.replace("/login");
    void userServiceLogout({}).catch((err) => {
      console.warn("logout revoke failed", err);
    });
  }

  const rowStyle = useCallback(
    (key: string) => [
      styles.row,
      {
        backgroundColor: focusedItem === key ? theme.colors.accent + "33" : "transparent",
        borderRadius: theme.radii.md,
      },
    ],
    [focusedItem, theme],
  );

  return (
    <ScrollView style={[styles.container, { backgroundColor: theme.colors.bg }]} contentContainerStyle={{ padding: 32 }}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.settings")}</Text>

      {/* Server */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("mobile.settings.server")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <View style={styles.infoRow}>
          {/* mobile.serverConnect.urlLabel, not admin.webhooks.url: this row is
              the server this TV is paired with, so it must not inherit whatever
              wording the admin webhook form settles on. */}
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("mobile.serverConnect.urlLabel", "Server URL")}</Text>
          <Text style={[styles.value, { color: theme.colors.textMuted }]}>{serverUrl ?? "..."}</Text>
        </View>
        <Pressable
          style={rowStyle("change-server")}
          onPress={handleChangeServer}
          onFocus={() => setFocusedItem("change-server")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("mobile.settings.changeServer")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("mobile.settings.changeServer")}</Text>
        </Pressable>
      </View>

      {/* Account */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("auth.settings.tabs.account")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <View style={styles.infoRow}>
          {/* auth.login.username, not admin.users.user: this is the signed-in
              account on this TV, not a row in the admin user table. */}
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("auth.login.username", "Username")}</Text>
          <Text style={[styles.value, { color: theme.colors.textMuted }]}>{user?.username ?? "..."}</Text>
        </View>
        <Pressable
          style={rowStyle("switch-profile")}
          onPress={() => router.push("/profile-select")}
          onFocus={() => setFocusedItem("switch-profile")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("mobile.settings.switchProfile")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("mobile.settings.switchProfile")}</Text>
        </Pressable>
        <Pressable
          style={rowStyle("sign-out")}
          onPress={handleSignOut}
          onFocus={() => setFocusedItem("sign-out")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("mobile.settings.signOut")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("mobile.settings.signOut")}</Text>
        </Pressable>
      </View>

      {/* Watch History */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("common.activity")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <Pressable
          style={rowStyle("watch-history")}
          onPress={() => router.push("/(auth)/history")}
          onFocus={() => setFocusedItem("watch-history")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("auth.watchHistory.title")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("auth.watchHistory.title")}</Text>
        </Pressable>
        <Pressable
          style={rowStyle("notifications")}
          onPress={() => router.push("/(auth)/notifications")}
          onFocus={() => setFocusedItem("notifications")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("common.notifications.title")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.notifications.title")}</Text>
        </Pressable>
      </View>

      {/* Playback */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("auth.settings.tabs.playback")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <Pressable
          style={rowStyle("playback")}
          onPress={() => router.push("/(auth)/settings/playback")}
          onFocus={() => setFocusedItem("playback")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("auth.settings.tabs.playback")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("auth.settings.tabs.playback")}</Text>
        </Pressable>
      </View>

      {/* Language */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("common.language")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {supportedLanguages.map((lang) => (
          <Pressable
            key={lang.code}
            style={rowStyle(`lang-${lang.code}`)}
            onPress={() => handleLanguageChange(lang.code)}
            onFocus={() => setFocusedItem(`lang-${lang.code}`)}
            onBlur={() => setFocusedItem(null)}
            accessible={true}
            accessibilityLabel={lang.label}
            accessibilityRole="button"
          >
            <Text style={[styles.label, { color: theme.colors.text }]}>{lang.label}</Text>
            {currentLang === lang.code && (
              <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{"✓"}</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* Appearance */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("auth.settings.tabs.appearance")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {/* The a11y label is "<name> theme", not just "<name>": on a remote the
            reader announces this row with no surrounding context, and "AMOLED"
            alone does not say what it selects. */}
        {THEME_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={rowStyle(`theme-${opt.value}`)}
            onPress={() => setTheme(opt.value)}
            onFocus={() => setFocusedItem(`theme-${opt.value}`)}
            onBlur={() => setFocusedItem(null)}
            accessible={true}
            accessibilityLabel={t("mobile.settings.themeOptionA11y", "{{name}} theme", { name: t(opt.labelKey, opt.fallback) })}
            accessibilityRole="button"
          >
            <Text style={[styles.label, { color: theme.colors.text }]}>{t(opt.labelKey, opt.fallback)}</Text>
            {currentTheme === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{"✓"}</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* About */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("mobile.settings.about")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <View style={styles.infoRow}>
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.serverVersion")}</Text>
          <Text style={[styles.value, { color: theme.colors.textMuted }]}>{systemInfo?.version ?? "-"}</Text>
        </View>
      </View>

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  header: { fontSize: 36, fontWeight: "700", marginBottom: 24 },
  sectionTitle: {
    fontSize: 16,
    fontWeight: "600",
    textTransform: "uppercase",
    marginTop: 24,
    marginBottom: 8,
  },
  section: {
    borderRadius: 12,
    overflow: "hidden",
    padding: 4,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 16,
    paddingHorizontal: 20,
  },
  infoRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 16,
    paddingHorizontal: 20,
  },
  label: {
    fontSize: 20,
  },
  value: {
    fontSize: 18,
    flexShrink: 1,
    marginLeft: 16,
  },
});
