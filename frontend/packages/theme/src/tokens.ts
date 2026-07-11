// Design tokens for the mobile (React Native) client. These are DERIVED from
// themeVars.ts - the single source of truth shared with the web CSS - so a
// colour defined once drives both platforms and cannot drift. Only genuinely
// mobile-only concepts (error tint, shadow colour, spacing scale - none of
// which has a web CSS var) are specified here.
import { themeVars, type ThemeCSSName } from "./themeVars";

export interface ThemeTokens {
  colors: {
    bg: string;
    surface: string;
    surfaceRaised: string;
    accent: string;
    accentHover: string;
    text: string;
    textMuted: string;
    border: string;
    error: string;
    danger: string;
    dangerHover: string;
    onAccent: string;
    overlay: string;
    divider: string;
    shadow: string;
    success: string;
    warning: string;
    info: string;
  };
  radii: { sm: number; md: number; lg: number };
  spacing: { xs: number; sm: number; md: number; lg: number; xl: number };
}

// error is a mobile-only surface tint with no web CSS var, so it stays here.
const MOBILE_ERROR: Record<ThemeCSSName, string> = {
  "default": "#e57373",
  "amoled": "#f87171",
  "dark-red": "#f87171",
  "light-default": "#ef4444",
  "light-blue": "#dc2626",
};

// The spacing scale is a native-only concept (web uses Tailwind's scale, not
// CSS vars), identical across themes.
const SPACING = { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 } as const;

// resolveVar unwraps a `var(--x)` reference to its concrete value (RN can't
// resolve CSS variables). --danger/--success/--warning alias the --color-*
// tokens in every theme.
function resolveVar(vars: Record<string, string>, value: string): string {
  const m = value.match(/^var\((--[a-z0-9-]+)\)$/i);
  return m ? (vars[m[1]] ?? value) : value;
}

const px = (v: string): number => parseInt(v, 10);

export function deriveTokens(name: ThemeCSSName): ThemeTokens {
  const v = themeVars[name];
  return {
    colors: {
      bg: v["--bg"],
      surface: v["--surface"],
      surfaceRaised: v["--surface-raised"],
      accent: v["--accent"],
      accentHover: v["--accent-hover"],
      text: v["--text"],
      textMuted: v["--text-muted"],
      border: v["--border"],
      error: MOBILE_ERROR[name],
      danger: resolveVar(v, v["--danger"]),
      dangerHover: v["--danger-hover"],
      onAccent: v["--filter-active-text"],
      overlay: v["--overlay-bg"],
      divider: v["--glass-border"],
      shadow: "#000",
      success: resolveVar(v, v["--success"]),
      warning: resolveVar(v, v["--warning"]),
      info: v["--accent"],
    },
    radii: { sm: px(v["--radius-sm"]), md: px(v["--radius-md"]), lg: px(v["--radius-lg"]) },
    spacing: { ...SPACING },
  };
}

export const defaultTheme: ThemeTokens = deriveTokens("default");
export const amoledTheme: ThemeTokens = deriveTokens("amoled");
export const lightDefaultTheme: ThemeTokens = deriveTokens("light-default");
export const darkRedTheme: ThemeTokens = deriveTokens("dark-red");
export const lightBlueTheme: ThemeTokens = deriveTokens("light-blue");

export const themes = {
  default: defaultTheme,
  amoled: amoledTheme,
  "dark-red": darkRedTheme,
  "light-default": lightDefaultTheme,
  "light-blue": lightBlueTheme,
} as const;

export type ThemeName = keyof typeof themes;
