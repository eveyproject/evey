import React, { createContext, useContext, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { themes, defaultTheme, onColor, type ThemeTokens, type ThemeName } from "@evey/theme";
import { useThemeStore } from "@evey/store";

const ThemeContext = createContext<ThemeTokens>(defaultTheme);

export function useTheme() {
  return useContext(ThemeContext);
}

const PLUGIN_PREFIX = "plugin:";

interface PluginThemeInfo {
  id: string;
  name: string;
  tokens?: Record<string, string>;
}

// Mapping from theme.css custom-property names to the mobile ThemeTokens
// shape. Keep this in sync with the contract documented at the top of
// `dark-blue-theme/theme.css`. Only colors are overridden; radii and
// spacing keep the platform defaults.
export function mergeCSSVars(base: ThemeTokens, vars: Record<string, string>): ThemeTokens {
  const get = (k: string, fallback: string) => vars[k] ?? fallback;
  // onSuccess/onWarning are DERIVED from the fill they sit on, so they have to
  // be recomputed from the plugin's fill - keeping the base value would paint
  // dark-theme text on a plugin's light fill.
  const success = get("--success", base.colors.success);
  const warning = get("--warning", base.colors.warning);
  return {
    ...base,
    colors: {
      ...base.colors,
      bg:            get("--bg",              base.colors.bg),
      surface:       get("--surface",         base.colors.surface),
      surfaceRaised: get("--surface-raised",  base.colors.surfaceRaised),
      accent:        get("--accent",          base.colors.accent),
      accentHover:   get("--accent-hover",    base.colors.accentHover),
      text:          get("--text",            base.colors.text),
      textMuted:     get("--text-muted",      base.colors.textMuted),
      border:        get("--border",          base.colors.border),
      // overlay/divider fall back to base when a plugin theme doesn't supply
      // them (today's themes don't), but a theme that does will now be honored
      // instead of silently rendering the default - both are consumed widely
      // (overlay in ~14 screens, divider in ~5).
      overlay:       get("--overlay",          base.colors.overlay),
      divider:       get("--divider",          base.colors.divider),
      error:         get("--error",           base.colors.error),
      danger:        get("--color-danger",    base.colors.danger),
      success,
      warning,
      info:          get("--accent",          base.colors.info),
      onAccent:      vars["--filter-active-text"] ?? vars["--overlay-text"] ?? base.colors.onAccent,
      onSuccess:     onColor(success),
      onWarning:     onColor(warning),
      onDanger:      get("--on-danger",       base.colors.onDanger),
      shadow:        get("--shadow",          base.colors.shadow),
    },
  };
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const themeName = useThemeStore((s) => s.theme);
  const pluginId = useThemeStore((s) => s.pluginId);
  const isPluginTheme = themeName.startsWith(PLUGIN_PREFIX);

  const { data: pluginThemes } = useQuery<PluginThemeInfo[]>({
    queryKey: ["plugin-themes"],
    queryFn: async () => {
      const data = await eveyFetch<{ themes: PluginThemeInfo[] }>({
        url: "/api/v1/plugins/themes",
        method: "GET",
      });
      return data.themes ?? [];
    },
    enabled: isPluginTheme,
    staleTime: 60_000,
  });

  const tokens = useMemo(() => {
    if (isPluginTheme && pluginId) {
      const entry = pluginThemes?.find((t) => t.id === pluginId);
      if (entry?.tokens) return mergeCSSVars(defaultTheme, entry.tokens);
    }
    if (isPluginTheme) {
      // Plugin theme selected but tokens not yet loaded: hold the default
      // until the query resolves rather than flashing back to a builtin.
      return defaultTheme;
    }
    return themes[themeName as ThemeName] ?? defaultTheme;
  }, [themeName, pluginId, pluginThemes, isPluginTheme]);

  return (
    <ThemeContext.Provider value={tokens}>{children}</ThemeContext.Provider>
  );
}
