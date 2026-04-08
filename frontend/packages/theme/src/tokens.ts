// Design tokens — single source of truth for all platforms
// Web: converted to CSS custom properties
// Native (RN): consumed directly as JS values via useTheme()

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
  };
  radii: { sm: number; md: number; lg: number };
  spacing: { xs: number; sm: number; md: number; lg: number; xl: number };
}

export const defaultTheme: ThemeTokens = {
  colors: {
    bg:           "#0c1220",
    surface:      "#151d2e",
    surfaceRaised:"#1e293b",
    accent:       "#818cf8",
    accentHover:  "#6366f1",
    text:         "#f1f5f9",
    textMuted:    "#90a4bd",
    border:       "#1e293b",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const amoledTheme: ThemeTokens = {
  colors: {
    bg:           "#000000",
    surface:      "#0a0a0a",
    surfaceRaised:"#141414",
    accent:       "#c4b5fd",
    accentHover:  "#a78bfa",
    text:         "#e4e4e7",
    textMuted:    "#71717a",
    border:       "#1a1a1a",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const lightDefaultTheme: ThemeTokens = {
  colors: {
    bg:           "#f8f9fb",
    surface:      "#ffffff",
    surfaceRaised:"#eef0f4",
    accent:       "#6366f1",
    accentHover:  "#4f46e5",
    text:         "#0f172a",
    textMuted:    "#94a3b8",
    border:       "#e2e8f0",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const lightBlueTheme: ThemeTokens = {
  colors: {
    bg:           "#f0f4f8",
    surface:      "#f7f9fc",
    surfaceRaised:"#e2e8f0",
    accent:       "#3b82f6",
    accentHover:  "#2563eb",
    text:         "#0f172a",
    textMuted:    "#94a3b8",
    border:       "#cbd5e1",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const themes = {
  default:        defaultTheme,
  amoled:         amoledTheme,
  "light-default": lightDefaultTheme,
  "light-blue":    lightBlueTheme,
} as const;

export type ThemeName = keyof typeof themes;
