import { useEffect } from "react";
import { themeVars, THEME_NAMES, type ThemeCSSName } from "@evey/theme";
import { useThemeStore } from "@evey/store";
import { isPluginThemeName, useActiveProfile, usePluginThemes } from "@evey/client";

// The palette the web app reads from a generated stylesheet is set here as
// the same CSS variables, from the same source of truth. A built-in by
// name; a plugin's sheet laid over the default, exactly as the phone and
// the Android TV merge it into their tokens.
export function applyThemeVars(name: string, pluginVars?: Record<string, string>): void {
  const base = themeVars[(THEME_NAMES as readonly string[]).includes(name) ? (name as ThemeCSSName) : "default"];
  const root = document.documentElement;
  for (const [k, v] of Object.entries(base)) root.style.setProperty(k, v);
  if (pluginVars) for (const [k, v] of Object.entries(pluginVars)) root.style.setProperty(k, v);
}

/**
 * Keeps the page painted in the profile's theme: the store's choice (set
 * from the profile the moment it loads, as the settings screens on every
 * client do) drives the CSS variables, and a plugin theme fetches its sheet.
 */
export function useAppliedTheme(): void {
  const themeName = useThemeStore((s) => s.theme);
  const pluginId = useThemeStore((s) => s.pluginId);
  const applyFromPreferences = useThemeStore((s) => s.applyFromPreferences);
  const { profile } = useActiveProfile();
  const isPlugin = isPluginThemeName(themeName);
  const { data: pluginThemes } = usePluginThemes(isPlugin);

  useEffect(() => {
    applyFromPreferences(profile?.preferences);
  }, [profile?.preferences, applyFromPreferences]);

  useEffect(() => {
    if (isPlugin) {
      const entry = pluginThemes?.find((p) => p.id === pluginId);
      applyThemeVars("default", entry?.tokens);
      return;
    }
    applyThemeVars(themeName);
  }, [themeName, pluginId, isPlugin, pluginThemes]);
}
