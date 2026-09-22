import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { useUserServiceUpdateProfile } from "@evey/api";
import { useServerListStore, useThemeStore, themePreferences } from "@evey/store";
import { themes, THEME_NAMES } from "@evey/theme";
import { pluginThemeKey, useActiveProfile, usePluginThemes } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";
import { navigate, replace } from "../app/router";
import { signOut } from "../lib/session";
import i18n, { supportedLanguages, saveLocale } from "../i18n";

// The same five built-ins and the plugins' themes the phone, the browser and
// the Android TV offer, saved on the profile so the choice follows the viewer.
const THEME_LABELS: Record<string, { key: string; fallback: string }> = {
  default: { key: "mobile.settings.themeDark", fallback: "Dark" },
  amoled: { key: "mobile.settings.themeAmoled", fallback: "AMOLED" },
  "dark-red": { key: "mobile.settings.themeDarkRed", fallback: "Dark Red" },
  "light-default": { key: "mobile.settings.themeLight", fallback: "Light" },
  "light-blue": { key: "mobile.settings.themeLightBlue", fallback: "Light Blue" },
};

export function SettingsScreen() {
  const { t } = useTranslation();
  const server = useServerListStore((s) => s.getActiveServer());
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const { profile } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  const { data: pluginThemes } = usePluginThemes();

  // MERGED because the server replaces the whole preferences map (see
  // themePreferences): sending the theme alone would wipe the languages.
  const pickTheme = useCallback((value: string, pluginId?: string) => {
    setTheme(value, pluginId);
    if (!profile?.id) return;
    updateProfile.mutate({ id: profile.id, data: { preferences: themePreferences(profile.preferences, value, pluginId) } });
  }, [setTheme, updateProfile, profile]);

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("common.nav.settings", "Settings")}</h1>
      <p className="subtitle">{server?.label || server?.serverName || server?.url}</p>
      <h2 className="row-title">{t("common.language", "Language")}</h2>
      <div className="actions">
        {supportedLanguages.map((lng) => (
          <Button key={lng} variant={i18n.language === lng ? "primary" : "default"} onPress={() => { saveLocale(lng); void i18n.changeLanguage(lng); }}>
            {lng.toUpperCase()}
          </Button>
        ))}
      </div>
      <h2 className="row-title">{t("auth.settings.tabs.appearance", "Appearance")}</h2>
      <div className="actions" style={{ flexWrap: "wrap", justifyContent: "center" }}>
        {THEME_NAMES.map((name) => (
          <Button key={name} variant={currentTheme === name ? "primary" : "default"} onPress={() => pickTheme(name)}>
            <span className="swatch" style={{ background: themes[name].colors.accent }} />
            {t(THEME_LABELS[name].key, THEME_LABELS[name].fallback)}
          </Button>
        ))}
        {(pluginThemes ?? []).map((pt) => (
          <Button key={pt.id} variant={currentTheme === pluginThemeKey(pt.name) ? "primary" : "default"} onPress={() => pickTheme(pluginThemeKey(pt.name), pt.id)}>
            <span className="swatch" style={{ background: pt.tokens?.["--accent"] ?? "currentColor" }} />
            {pt.name}
          </Button>
        ))}
      </div>
      <h2 className="row-title">{t("common.nav.settings", "Settings")}</h2>
      <div className="actions" style={{ flexWrap: "wrap", justifyContent: "center" }}>
        <Button autoFocus onPress={() => navigate({ name: "playback" })}>{t("auth.settings.tabs.playback")}</Button>
        <Button onPress={() => navigate({ name: "stats" })}>{t("common.userStats.title")}</Button>
        <Button onPress={() => navigate({ name: "history" })}>{t("common.watchHistory.title")}</Button>
        <Button onPress={() => navigate({ name: "notifications" })}>{t("common.notifications.title")}</Button>
        <Button onPress={() => navigate({ name: "home" })}>{t("media.home.customize")}</Button>
        <Button onPress={() => navigate({ name: "servers" })}>{t("mobile.servers.manageTitle")}</Button>
      </div>
      <div className="actions">
        <Button onPress={() => replace({ name: "profiles" })}>{t("common.nav.switchProfile", "Switch profile")}</Button>
        <Button onPress={() => { void signOut().then(() => replace({ name: "login" })); }}>{t("common.nav.signOut", "Sign out")}</Button>
        <Button variant="ghost" onPress={() => { void signOut().then(() => replace({ name: "server" })); }}>{t("mobile.settings.changeServer", "Change server")}</Button>
      </div>
    </FocusRoot>
  );
}
