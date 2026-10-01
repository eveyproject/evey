import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useAdminSystemServiceGetSystemInfo, useUserServiceGetCurrentUser, useUserServiceUpdateProfile } from "@evey/api";
import { useAuthStore, useThemeStore, themePreferences, isDemoMode } from "@evey/store";
import { themes } from "@evey/theme";
import { UI_SCALE_OPTIONS, clearUnlocked, pluginThemeKey, resolveSlotEntries, useActiveProfile, usePluginList, usePluginThemes } from "@evey/client";
import { applyUiScale, storedUiScale } from "../lib/uiScale";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { Pressable } from "../components/ui";
import { SettingsGroup, SettingsRow } from "../components/SettingsList";
import { navigate, replace } from "../app/router";
import { activeServerUrl, authRefresh, signOut } from "../lib/session";
import { avatarSrc } from "../lib/media";
import i18n, { saveLocale } from "../i18n";

const THEME_OPTIONS = [
  { labelKey: "mobile.settings.themeDark", fallback: "Dark", value: "default" },
  { labelKey: "mobile.settings.themeAmoled", fallback: "AMOLED", value: "amoled" },
  { labelKey: "mobile.settings.themeDarkRed", fallback: "Dark Red", value: "dark-red" },
  { labelKey: "mobile.settings.themeLight", fallback: "Light", value: "light-default" },
  { labelKey: "mobile.settings.themeLightBlue", fallback: "Light Blue", value: "light-blue" },
] as const;

const LANGUAGES = [
  { code: "en", label: "English" },
  { code: "de", label: "Deutsch" },
  { code: "es", label: "Español" },
  { code: "fr", label: "Français" },
  { code: "hi", label: "हिन्दी" },
  { code: "zh", label: "中文" },
] as const;

function ConfirmServerChange({ onCancel, onConfirm }: { onCancel: () => void; onConfirm: () => void }) {
  const { t } = useTranslation();
  useBackHandler(onCancel);
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus boundary className="confirm-box">
        <h2 className="confirm-title">{t("mobile.settings.changeServer")}</h2>
        <p className="confirm-body">{t("mobile.settings.changeServerConfirm", "Changing server signs you out of this television. Continue?")}</p>
        <div className="confirm-actions">
          <Pressable className="confirm-btn" autoFocus onPress={onCancel}>{t("common.cancel")}</Pressable>
          <Pressable className="confirm-btn primary" onPress={onConfirm}>{t("mobile.settings.changeServer")}</Pressable>
        </div>
      </FocusRoot>
    </div>
  );
}

/** The Settings tab, section by section as on the Android television. */
export function SettingsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: user } = useUserServiceGetCurrentUser();
  const { profile } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  const { data: pluginThemes } = usePluginThemes();
  const { data: systemInfo } = useAdminSystemServiceGetSystemInfo({ query: { enabled: !!isAdmin } });
  const [scale, setScale] = useState(() => storedUiScale());
  const [confirmServer, setConfirmServer] = useState(false);
  const [lang, setLang] = useState(i18n.language);
  const { data: pluginList } = usePluginList();
  const pluginEntries = resolveSlotEntries(pluginList?.plugins, "settings.user.appearance")
    .map((m) => ({ id: m.pluginId, name: (pluginList?.plugins ?? []).find((p) => p.id === m.pluginId)?.name ?? m.pluginId }));

  // MERGED because the server replaces the whole preferences map (see
  // themePreferences): sending the theme alone would wipe the languages.
  const pickTheme = useCallback((value: string, pluginId?: string) => {
    setTheme(value, pluginId);
    // Demo: the profile is shared, the theme stays on this television.
    if (!profile?.id || isDemoMode()) return;
    updateProfile.mutate({ id: profile.id, data: { preferences: themePreferences(profile.preferences, value, pluginId) } });
  }, [setTheme, updateProfile, profile]);

  // The profile keeps the language too, and a refreshed token carries it.
  const pickLanguage = async (code: string) => {
    saveLocale(code);
    setLang(code);
    void i18n.changeLanguage(code);
    if (!profile?.id || isDemoMode()) return;
    try {
      await updateProfile.mutateAsync({ id: profile.id, data: { locale: code } });
      await authRefresh.forceRefresh();
    } catch {
      /* the local choice stands; the profile catches up next time */
    }
  };

  const leave = (to: "login" | "server") => {
    setConfirmServer(false);
    void signOut().then(() => {
      qc.clear();
      clearUnlocked();
      replace({ name: to });
    });
  };

  const initial = (profile?.name ?? user?.username ?? "?").slice(0, 1).toUpperCase();

  return (
    <>
      <FocusRoot autoFocus className="tab-page set-page">
        <h1 className="set-title">{t("common.settings")}</h1>

        <SettingsGroup title={t("mobile.settings.server")}>
          <div className="set-info">
            <span className="set-label">{t("mobile.serverConnect.urlLabel", "Server URL")}</span>
            <span className="set-value">{activeServerUrl() ?? "..."}</span>
          </div>
          <SettingsRow label={t("mobile.settings.changeServer")} muted autoFocus onPress={() => setConfirmServer(true)} />
        </SettingsGroup>

        <SettingsGroup title={t("auth.settings.tabs.account")}>
          <div className="set-profile">
            <span className="set-avatar">{profile?.avatarPath && profile.id ? <img src={avatarSrc(profile.id)} alt="" /> : initial}</span>
            <span className="set-profile-names">
              <span className="set-profile-name">{profile?.name ?? t("auth.profile.title", "Profile")}</span>
              <span className="set-value">{user?.username ?? "..."}</span>
            </span>
          </div>
          <SettingsRow label={t("mobile.settings.switchProfile")} onPress={() => navigate({ name: "profiles", pick: true })} />
          <SettingsRow label={t("mobile.settings.signOut")} muted onPress={() => leave("login")} />
        </SettingsGroup>

        <SettingsGroup title={t("common.activity")}>
          <SettingsRow label={t("auth.watchHistory.title")} onPress={() => navigate({ name: "history" })} />
          <SettingsRow label={t("common.nav.myStats", "My statistics")} onPress={() => navigate({ name: "stats" })} />
          <SettingsRow label={t("common.notifications.title")} onPress={() => navigate({ name: "notifications" })} />
        </SettingsGroup>

        <SettingsGroup title={t("media.syncplay.title", "Watch together")}>
          <SettingsRow label={t("common.nav.joinRoom", "Join a room")} onPress={() => navigate({ name: "syncplay" })} />
        </SettingsGroup>

        <SettingsGroup title={t("auth.settings.tabs.playback")}>
          <SettingsRow label={t("auth.settings.tabs.playback")} onPress={() => navigate({ name: "playback" })} />
        </SettingsGroup>

        <SettingsGroup title={t("common.nav.home")}>
          <SettingsRow label={t("common.feeds.customize")} onPress={() => navigate({ name: "home", customize: true })} />
        </SettingsGroup>

        <SettingsGroup title={t("common.language")}>
          {LANGUAGES.map((l) => (
            <SettingsRow key={l.code} label={l.label} checked={lang === l.code} onPress={() => void pickLanguage(l.code)} />
          ))}
        </SettingsGroup>

        <SettingsGroup title={t("auth.settings.tabs.appearance")}>
          {THEME_OPTIONS.map((opt) => {
            const name = t(opt.labelKey, opt.fallback);
            return (
              <SettingsRow key={opt.value} label={t("mobile.settings.themeOptionA11y", { name, defaultValue: `${name} theme` })} checked={currentTheme === opt.value} onPress={() => pickTheme(opt.value)}>
                <span className="set-theme"><span className="set-swatch" style={{ background: themes[opt.value].colors.accent }} /><span className="set-label">{name}</span></span>
              </SettingsRow>
            );
          })}
          {(pluginThemes ?? []).map((pt) => {
            const key = pluginThemeKey(pt.name);
            return (
              <SettingsRow key={key} label={t("mobile.settings.themeOptionA11y", { name: pt.name, defaultValue: `${pt.name} theme` })} checked={currentTheme === key} onPress={() => pickTheme(key, pt.id)}>
                <span className="set-theme"><span className="set-swatch" style={{ background: pt.tokens?.["--accent"] ?? "var(--accent)" }} /><span className="set-label">{pt.name}</span></span>
              </SettingsRow>
            );
          })}
        </SettingsGroup>

        <SettingsGroup title={t("mobile.settings.uiScale", "Interface size")}>
          {UI_SCALE_OPTIONS.map((pct) => (
            <SettingsRow key={pct} label={`${pct} %`} checked={scale === pct} onPress={() => { setScale(pct); applyUiScale(pct); }} />
          ))}
        </SettingsGroup>

        {pluginEntries.length > 0 ? (
          <SettingsGroup title={t("auth.settings.tabs.plugins")}>
            {pluginEntries.map((p) => (
              <SettingsRow key={p.id} label={p.name} onPress={() => navigate({ name: "plugin-settings", pluginId: p.id })} />
            ))}
          </SettingsGroup>
        ) : null}

        {isAdmin ? (
          <>
            <h2 className="set-section-title">{t("mobile.settings.about")}</h2>
            <div className="set-section">
              <div className="set-info">
                <span className="set-label">{t("common.serverVersion")}</span>
                <span className="set-value">{systemInfo?.version ?? "-"}</span>
              </div>
            </div>
          </>
        ) : null}
      </FocusRoot>
      {confirmServer ? <ConfirmServerChange onCancel={() => setConfirmServer(false)} onConfirm={() => leave("server")} /> : null}
    </>
  );
}
