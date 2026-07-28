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
    // Text painted ON a success/warning fill (chips, badges). No theme
    // declares an on-colour for these two (unlike --filter-active-text for
    // the accent and --on-danger), so they are DERIVED: whichever of near
    // black / white contrasts better with the fill, per theme.
    onSuccess: string;
    onWarning: string;
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

// WCAG relative luminance of an sRGB hex - the same math contrast.test.ts
// asserts the palette with. Non-hex input (a var() that failed to resolve)
// reports mid-luminance so the caller's pick degrades to white-on-colour.
function luminance(hex: string): number {
  const h = hex.replace("#", "");
  const full = h.length === 3 ? h.split("").map((c) => c + c).join("") : h;
  if (!/^[0-9a-fA-F]{6}$/.test(full)) return 0.2;
  const [r, g, b] = [0, 2, 4]
    .map((i) => parseInt(full.slice(i, i + 2), 16) / 255)
    .map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// onColor picks the readable text colour for a filled chip: near-black or
// white, whichever has the higher WCAG contrast ratio against the fill.
export function onColor(fill: string): string {
  const lum = luminance(fill);
  const dark = 0.005; // luminance of #0a0a0a
  const ratioDark = (lum + 0.05) / (dark + 0.05);
  const ratioWhite = 1.05 / (lum + 0.05);
  return ratioDark >= ratioWhite ? "#0a0a0a" : "#fff";
}

// Type and icon scales, in px, shared by web / mobile / TV.
//
// These MIRROR the web ladder declared in apps/web/src/styles/main.css's
// @theme block (--text-xs … --text-4xl) and apps/web/src/constants/iconSizes.ts.
// Web reads its own copy because Tailwind needs the values at build time; the
// numbers here are what mobile and TV consume, and the two must stay equal -
// that is the whole point of writing them down once.
//
// They exist because the native clients had no ladder at all: the mobile app
// carried 137 hardcoded `fontSize:` values across 12 distinct sizes, which is
// how a "small" label ended up being 11, 12 or 13 depending on the screen.
export const FONT_SIZE = {
  xs: 13,
  sm: 15,
  base: 16,
  lg: 18,
  xl: 20,
  "2xl": 24,
  "3xl": 30,
  "4xl": 36,
} as const;

export const ICON_SIZE = {
  xs: 12,
  sm: 14,
  md: 16,
  lg: 20,
  xl: 24,
  "2xl": 32,
  "3xl": 40,
  "4xl": 48,
} as const;

export type FontSizeStep = keyof typeof FONT_SIZE;
export type IconSizeStep = keyof typeof ICON_SIZE;

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
      onSuccess: onColor(resolveVar(v, v["--success"])),
      onWarning: onColor(resolveVar(v, v["--warning"])),
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
