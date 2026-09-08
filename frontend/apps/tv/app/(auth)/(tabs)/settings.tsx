import { useState, useCallback, useEffect } from "react";
import { View, Text, ScrollView, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, useAdminSystemServiceGetSystemInfo, userServiceLogout, eveyFetch } from "@evey/api";
import { useAuthStore, useThemeStore, useLocaleStore, themePreferences } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { clearServerUrl } from "../../../src/lib/server-config";
import { queryClient } from "../../../src/lib/query-client";
import { clearRefreshToken } from "../../../src/lib/refresh-token";
import { clearUnlocked } from "../../../src/hooks/useParentalGate";
import { supportedLanguages } from "../../../src/i18n";
import { usePluginSlotEntries } from "../../../src/features/plugins/PluginSlot";

// labelKey/fallback pairs mirror mobile's ThemeSelector.tsx builtInI18nKeys
// mapping - same three built-in themes this TV settings screen exposes.
const THEME_OPTIONS = [
  { labelKey: "mobile.settings.themeDark", fallback: "Dark", value: "default" },
  { labelKey: "mobile.settings.themeLight", fallback: "Light", value: "light-default" },
  { labelKey: "mobile.settings.themeAmoled", fallback: "AMOLED", value: "amoled" },
] as const;

export default function SettingsScreen() {
  const theme = useTheme();
  // Plugins mounting the only user-settings slot the server allows.
  const pluginEntries = usePluginSlotEntries("settings.user.appearance");
  const { t, i18n } = useTranslation();
  const logout = useAuthStore((s) => s.logout);
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const applyThemeFromPreferences = useThemeStore((s) => s.applyFromPreferences);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const serverUrl = useServerUrlNullable();

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p: any) => p.isDefault) ?? profilesResp?.profiles?.[0];
  // The profile actually in use on this television, which is not necessarily
  // the default one - somebody switched to it from profile-select.
  const activeProfileId = useAuthStore((s) => s.profileId);
  const activeProfile =
    profilesResp?.profiles?.find((p: { id?: string }) => p.id === activeProfileId) ?? defaultProfile;
  const updateProfile = useUserServiceUpdateProfile();
  const accessToken = useAuthStore((s) => s.accessToken);
  const authHeaders = useAuthHeaders();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  // Non-admin users get a bare 403 on every visit to this screen for a piece
  // of information (server version) they cannot even use - gate the call on
  // the role instead of eating a failed request each time.
  const { data: systemInfo } = useAdminSystemServiceGetSystemInfo({ query: { enabled: isAdmin } });

  const [focusedItem, setFocusedItem] = useState<string | null>(null);
  const currentLang = i18n.language;

  // The theme belongs to the profile, not to this television: pick it on the
  // phone and the TV should follow. Persisted through the preferences map,
  // MERGED because the server replaces it wholesale (see themePreferences).
  const handleThemeChange = useCallback(
    (value: string) => {
      setTheme(value);
      if (!activeProfile?.id) return;
      updateProfile.mutate({
        id: activeProfile.id,
        data: { preferences: themePreferences(activeProfile.preferences, value) },
      });
    },
    [setTheme, updateProfile, activeProfile],
  );

  // And the other direction: adopt whatever the profile carries.
  useEffect(() => {
    applyThemeFromPreferences(activeProfile?.preferences);
  }, [activeProfile?.preferences, applyThemeFromPreferences]);

  async function handleLanguageChange(code: string) {
    setLocale(code);
    void i18n.changeLanguage(code);
    // The ACTIVE profile, not the default one. This screen already worked out
    // which profile is in use (see above) and then wrote the language onto
    // somebody else's - switch to a second profile, change the language, and
    // the default profile silently changed instead.
    if (activeProfile?.id) {
      try {
        await updateProfile.mutateAsync({ id: activeProfile.id, data: { locale: code } });
        const resp = await eveyFetch<{ accessToken?: string }>({ url: "/api/v1/auth/refresh", method: "POST", data: {} });
        if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      } catch {
        // The locale was already applied in-app; the server-side persist
        // failure is best-effort and surfaces on next refresh.
      }
    }
  }

  async function handleChangeServer() {
    // Same local teardown as sign-out: the cache holds the previous account's
    // rows and the refresh token would cold-start it back. queryClient.clear()
    // also fires via the active-server subscription, but clearing here keeps
    // this path self-contained.
    queryClient.clear();
    clearUnlocked();
    logout();
    void clearRefreshToken().catch((err) => {
      console.warn("failed to clear refresh token on server change", err);
    });
    await clearServerUrl();
    router.replace("/server-connect");
  }

  function handleSignOut() {
    // Clear local state and navigate FIRST: on a shared/offline TV, sign-out
    // must never appear to hang, or worse, silently stay logged in, waiting
    // on a network call. The server-side revoke is best-effort and its
    // failure must never be allowed to leave the refresh/stream tokens alive
    // while the UI already shows the login screen.
    //
    // A shared television is the whole point here: query keys carry no
    // identity, so without clearing the cache the NEXT viewer is served the
    // previous account's continue-watching, history and recommendations from
    // react-query's 5-minute window; and a refresh token left on disk cold-
    // starts the session straight back. Clear both, and drop parental unlocks
    // so the next viewer starts locked.
    queryClient.clear();
    clearUnlocked();
    logout();
    router.replace("/login");
    void userServiceLogout({}).catch((err) => {
      console.warn("logout revoke failed", err);
    });
    void clearRefreshToken().catch((err) => {
      console.warn("failed to clear refresh token on logout", err);
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
          // Claim the first focus explicitly. Left to Android, entering
          // Settings landed on "Log out", one reflex press away from a session
          // a television has no comfortable way to restore (device, 08-03).
          hasTVPreferredFocus
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
        {/* Who is watching, first: on a shared television the active PROFILE
            is the thing that decides what the home shows and what the parental
            gate allows, and it was nowhere on this screen. The account name
            stays underneath - it is what you sign out of. */}
        <View style={styles.profileHeader}>
          <View style={[styles.avatar, { backgroundColor: theme.colors.accent, overflow: "hidden" }]}>
            {activeProfile?.avatarPath && activeProfile.id && serverUrl && accessToken ? (
              // Token travels in the Authorization header, not the query
              // string: a query-string token leaks into reverse-proxy logs.
              <Image
                source={{ uri: `${serverUrl}/api/v1/profiles/${activeProfile.id}/avatar`, headers: authHeaders }}
                style={styles.avatarImage}
                contentFit="cover"
                accessible={false}
              />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontSize: 30, fontWeight: "700" }}>
                {(activeProfile?.name ?? user?.username ?? "?").slice(0, 1).toUpperCase()}
              </Text>
            )}
          </View>
          <View style={{ flex: 1 }}>
            <Text style={[styles.profileName, { color: theme.colors.text }]} numberOfLines={1}>
              {activeProfile?.name ?? t("auth.profile.title", "Profile")}
            </Text>
            <Text style={[styles.value, { color: theme.colors.textMuted }]}>{user?.username ?? "..."}</Text>
          </View>
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
          style={rowStyle("my-stats")}
          onPress={() => router.push("/(auth)/settings/stats")}
          onFocus={() => setFocusedItem("my-stats")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("common.nav.myStats", "My statistics")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.nav.myStats", "My statistics")}</Text>
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

      {/* Session - the things that involve another device or another viewer.
          Syncplay already had a screen on this TV and no way in: the room you
          join is started from a phone or a browser, so the TV only ever needs
          the join half, and it was unreachable. */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("media.syncplay.title", "Watch together")}</Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        <Pressable
          style={rowStyle("join-room")}
          onPress={() => router.push("/(auth)/syncplay")}
          onFocus={() => setFocusedItem("join-room")}
          onBlur={() => setFocusedItem(null)}
          accessible={true}
          accessibilityLabel={t("common.nav.joinRoom", "Join a room")}
          accessibilityRole="button"
        >
          <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.nav.joinRoom", "Join a room")}</Text>
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
            onPress={() => handleThemeChange(opt.value)}
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

      {/* Plugins that ship their own per-profile settings. They used to be
          injected inline here as a single raw WebView, with nothing saying
          which plugin owned it. Now each gets a menu row and a screen of its
          own; no plugin installed means no section at all - same parity fix
          as web/mobile. */}
      {pluginEntries.length > 0 && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("auth.settings.tabs.plugins")}</Text>
          <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
            {pluginEntries.map((entry) => (
              <Pressable
                key={entry.pluginId}
                style={rowStyle(`plugin-${entry.pluginId}`)}
                onPress={() => router.push(`/(auth)/settings/plugins/${entry.pluginId}`)}
                onFocus={() => setFocusedItem(`plugin-${entry.pluginId}`)}
                onBlur={() => setFocusedItem(null)}
                accessible={true}
                accessibilityLabel={entry.name}
                accessibilityRole="button"
              >
                <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>{entry.name}</Text>
              </Pressable>
            ))}
          </View>
        </>
      )}

      {/* About - the server version row is admin-only info (GetSystemInfo is an
          admin RPC): showing it to everyone else meant a 403 on every visit
          and a version that always read "-". */}
      {isAdmin && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("mobile.settings.about")}</Text>
          <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
            <View style={styles.infoRow}>
              <Text style={[styles.label, { color: theme.colors.text }]}>{t("common.serverVersion")}</Text>
              <Text style={[styles.value, { color: theme.colors.textMuted }]}>{systemInfo?.version ?? "-"}</Text>
            </View>
          </View>
        </>
      )}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  profileHeader: { flexDirection: "row", alignItems: "center", gap: 20, paddingVertical: 16, paddingHorizontal: 20 },
  avatar: { width: 64, height: 64, borderRadius: 32, justifyContent: "center", alignItems: "center" },
  avatarImage: { width: 64, height: 64 },
  profileName: { fontSize: 26, fontWeight: "700" },
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
