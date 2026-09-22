import React, { createContext, useContext, useMemo } from "react";
import { defaultTheme, type ThemeTokens } from "@evey/theme";
import { useThemeStore } from "@evey/store";
import { isPluginThemeName, mergeCSSVars, resolveThemeTokens, usePluginThemes } from "@evey/client";

const ThemeContext = createContext<ThemeTokens>(defaultTheme);

export function useTheme() {
  return useContext(ThemeContext);
}

// The CSS-variable to ThemeTokens mapping lives in @evey/client
// (lib/pluginTheme.ts) so the television paints a plugin theme exactly as the
// phone does; re-exported here for the callers and tests that import it.
export { mergeCSSVars };

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

  return (
    <ThemeContext.Provider value={tokens}>{children}</ThemeContext.Provider>
  );
}
