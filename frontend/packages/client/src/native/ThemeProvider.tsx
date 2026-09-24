import React, { createContext, useContext, useMemo } from "react";
import { defaultTheme, type ThemeTokens } from "@evey/theme";
import { useThemeStore } from "@evey/store";
import { isPluginThemeName, resolveThemeTokens } from "../lib/pluginTheme";
import { usePluginThemes } from "../hooks/usePluginThemes";

// The theme tokens of the two native apps, from one provider: the phone and
// the television kept a copy each. A profile on a plugin theme paints the
// plugin's sheet merged over the default; a built-in is resolved by name.
const ThemeContext = createContext<ThemeTokens>(defaultTheme);

export function useTheme() {
  return useContext(ThemeContext);
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const themeName = useThemeStore((s) => s.theme);
  const pluginId = useThemeStore((s) => s.pluginId);
  const isPluginTheme = isPluginThemeName(themeName);

  // Only asked for while a plugin theme is the choice: a built-in never
  // needs the server to paint.
  const { data: pluginThemes } = usePluginThemes(isPluginTheme);

  const tokens = useMemo(
    () => resolveThemeTokens(themeName, pluginId, pluginThemes),
    [themeName, pluginId, pluginThemes],
  );

  return <ThemeContext.Provider value={tokens}>{children}</ThemeContext.Provider>;
}
