import { defaultTheme, themes, onColor, type ThemeTokens, type ThemeName } from "@evey/theme";

// A theme a plugin ships is stored on the profile as "plugin:<name>" next to
// the plugin's id, so the three clients resolve the same choice the same way.
export const PLUGIN_THEME_PREFIX = "plugin:";

export interface PluginThemeInfo {
  id: string;
  name: string;
  tokens?: Record<string, string>;
}

export function isPluginThemeName(themeName: string): boolean {
  return themeName.startsWith(PLUGIN_THEME_PREFIX);
}

export function pluginThemeKey(name: string): string {
  return PLUGIN_THEME_PREFIX + name;
}

// Mapping from theme.css custom-property names to the native ThemeTokens
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

/**
 * The tokens to paint for a stored theme choice: a built-in by name, or a
 * plugin's sheet merged over the default. A plugin theme whose sheet has not
 * arrived yet holds the default rather than flashing back to a built-in.
 */
export function resolveThemeTokens(
  themeName: string,
  pluginId: string | null | undefined,
  pluginThemes: PluginThemeInfo[] | undefined,
): ThemeTokens {
  if (isPluginThemeName(themeName)) {
    if (pluginId) {
      const entry = pluginThemes?.find((t) => t.id === pluginId);
      if (entry?.tokens) return mergeCSSVars(defaultTheme, entry.tokens);
    }
    return defaultTheme;
  }
  return themes[themeName as ThemeName] ?? defaultTheme;
}
