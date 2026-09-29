import { useCallback, useEffect } from "react";
import { useThemeStore, themePreferences } from "@evey/store";
import { themes, type ThemeName } from "@evey/theme";
import { useTranslation } from "react-i18next";
import { useUserServiceUpdateProfile } from "@evey/api";
import { pluginThemeName } from "./theme-utils";
import { useActiveProfile } from "@/hooks/useActiveProfile";
import { useDemoMode } from "@/hooks/useDemoMode";
import { usePluginThemes } from "@evey/client";

const builtInLabels: Record<ThemeName, string> = {
  default: "Dark",
  amoled: "AMOLED",
  "dark-red": "Dark Red",
  "light-default": "Light",
  "light-blue": "Light Blue",
};

// The plugin themes come from @evey/client's usePluginThemes (the same query
// the phone and the televisions read).

export function ThemeSelector() {
  const { t } = useTranslation();
  const { theme, setTheme } = useThemeStore();
  const applyFromPreferences = useThemeStore((s) => s.applyFromPreferences);
  const { data: pluginThemes } = usePluginThemes();

  // The theme is a property of the profile, not of this browser: change it
  // here and the phone and the television follow, and the other way round.
  const { profile: activeProfile } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  // On the demo the profile is shared by every visitor: the theme stays in
  // this browser (the persisted store) and is neither read from nor written
  // to the profile - the server would refuse the write anyway.
  const demo = useDemoMode();

  useEffect(() => {
    applyFromPreferences(activeProfile?.preferences);
  }, [activeProfile?.preferences, applyFromPreferences]);

  // MERGED: UpdateProfile replaces the whole preferences map server side, so
  // sending only the theme would wipe the audio and subtitle languages.
  const pickTheme = useCallback(
    (name: string, pluginId?: string) => {
      // Built-in themes keep the one-argument call the store has always taken;
      // passing an explicit undefined would be equivalent but changes what
      // every caller and test sees.
      if (pluginId) setTheme(name, pluginId);
      else setTheme(name);
      if (!activeProfile?.id || demo) return;
      updateProfile.mutate({
        id: activeProfile.id,
        data: { preferences: themePreferences(activeProfile.preferences, name, pluginId) },
      });
    },
    [setTheme, updateProfile, activeProfile, demo],
  );

  return (
    <div className="flex flex-col gap-2" data-testid="theme-switcher">
      <label className="text-sm font-medium text-[var(--text-muted)]">{t("common.theme")}</label>
      <div className="flex gap-2 flex-wrap">
        {(Object.keys(themes) as ThemeName[]).map((name) => (
          <button
            key={name}
            data-testid={`theme-option-${name}`}
            onClick={() => pickTheme(name)}
            className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors-fast ${
              theme === name
                ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
                : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {builtInLabels[name]}
          </button>
        ))}
        {pluginThemes?.map((pt) => {
          const themeKey = pluginThemeName(pt.name);
          return (
            <button
              key={pt.id}
              data-testid={`theme-option-${themeKey}`}
              onClick={() => pickTheme(themeKey, pt.id)}
              className={`px-3 py-1.5 rounded-[var(--radius-md)] text-sm transition-colors-fast ${
                theme === themeKey
                  ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
                  : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
              }`}
            >
              {pt.name}
            </button>
          );
        })}
      </div>
    </div>
  );
}
