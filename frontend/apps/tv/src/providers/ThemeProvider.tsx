import React, { createContext, useContext, useMemo } from "react";
import { defaultTheme, type ThemeTokens } from "@evey/theme";
import { useThemeStore } from "@evey/store";
import { isPluginThemeName, resolveThemeTokens } from "@evey/client";
import { usePluginThemes } from "../hooks/usePluginThemes";

const ThemeContext = createContext<ThemeTokens>(defaultTheme);

export function useTheme() {
  return useContext(ThemeContext);
}

// The same resolution as the phone (lib/pluginTheme.ts in @evey/client): the
// five built-ins by name, and a plugin's sheet merged over the default. The
// television used to know the built-ins only, so a profile on a plugin theme
// painted the default here and the plugin's colours everywhere else.
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const themeName = useThemeStore((s) => s.theme);
  const pluginId = useThemeStore((s) => s.pluginId);
  const isPluginTheme = isPluginThemeName(themeName);
  const { data: pluginThemes } = usePluginThemes(isPluginTheme);

  const tokens = useMemo(
    () => resolveThemeTokens(themeName, pluginId, pluginThemes),
    [themeName, pluginId, pluginThemes],
  );

  return (
    <ThemeContext.Provider value={tokens}>{children}</ThemeContext.Provider>
  );
}
