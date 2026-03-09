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
    bg:           "#111827",
    surface:      "#1f2937",
    surfaceRaised:"#374151",
    accent:       "#6366f1",
    accentHover:  "#4f46e5",
    text:         "#f9fafb",
    textMuted:    "#9ca3af",
    border:       "#374151",
  },
  radii:   { sm: 4, md: 8, lg: 16 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const amoledTheme: ThemeTokens = {
  colors: {
    bg:           "#000000",
    surface:      "#0a0a0a",
    surfaceRaised:"#141414",
    accent:       "#bb86fc",
    accentHover:  "#9c5fd6",
    text:         "#e1e1e1",
    textMuted:    "#6b7280",
    border:       "#1f1f1f",
  },
  radii:   { sm: 4, md: 8, lg: 16 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const lightDefaultTheme: ThemeTokens = {
  colors: {
    bg:           "#f8f9fb",
    surface:      "#ffffff",
    surfaceRaised:"#eef0f4",
    accent:       "#6366f1",
    accentHover:  "#4f46e5",
    text:         "#111827",
    textMuted:    "#6b7280",
    border:       "#d1d5db",
  },
  radii:   { sm: 4, md: 8, lg: 16 },
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
    textMuted:    "#64748b",
    border:       "#cbd5e1",
  },
  radii:   { sm: 4, md: 8, lg: 16 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const themes = {
  default:        defaultTheme,
  amoled:         amoledTheme,
  "light-default": lightDefaultTheme,
  "light-blue":    lightBlueTheme,
} as const;

export type ThemeName = keyof typeof themes;
