import React, { createContext, useContext, useMemo } from "react";
import { themes, defaultTheme, type ThemeTokens, type ThemeName } from "@evey/theme";
import { useThemeStore } from "@evey/store";

const ThemeContext = createContext<ThemeTokens>(defaultTheme);

export function useTheme() {
  return useContext(ThemeContext);
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const themeName = useThemeStore((s) => s.theme);
  const tokens = useMemo(
    () => themes[themeName as ThemeName] ?? defaultTheme,
    [themeName],
  );

  return (
    <ThemeContext.Provider value={tokens}>{children}</ThemeContext.Provider>
  );
}
