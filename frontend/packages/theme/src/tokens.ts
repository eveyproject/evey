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
    error:        "#e57373",
    danger:       "#dc2626",
    dangerHover:  "#b91c1c",
    onAccent:     "#ffffff",
    overlay:      "rgba(0,0,0,0.5)",
    divider:      "rgba(255,255,255,0.08)",
    shadow:       "#000",
    success:      "#22c55e",
    warning:      "#f59e0b",
    info:         "#818cf8",
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
    error:        "#f87171",
    danger:       "#dc2626",
    dangerHover:  "#b91c1c",
    onAccent:     "#ffffff",
    overlay:      "rgba(0,0,0,0.7)",
    divider:      "rgba(255,255,255,0.06)",
    shadow:       "#000",
    success:      "#22c55e",
    warning:      "#f59e0b",
    info:         "#c4b5fd",
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
    error:        "#ef4444",
    danger:       "#dc2626",
    dangerHover:  "#b91c1c",
    onAccent:     "#ffffff",
    overlay:      "rgba(0,0,0,0.4)",
    divider:      "rgba(0,0,0,0.08)",
    shadow:       "#000",
    success:      "#22c55e",
    warning:      "#f59e0b",
    info:         "#6366f1",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const darkRedTheme: ThemeTokens = {
  colors: {
    bg:           "#000000",
    surface:      "#0d0d0d",
    surfaceRaised:"#181818",
    accent:       "#b81d24",
    accentHover:  "#e50914",
    text:         "#f5f5f5",
    textMuted:    "#8c8c8c",
    border:       "#1f1f1f",
    error:        "#f87171",
    danger:       "#b81d24",
    dangerHover:  "#831010",
    onAccent:     "#ffffff",
    overlay:      "rgba(0,0,0,0.6)",
    divider:      "rgba(255,255,255,0.06)",
    shadow:       "#000",
    success:      "#22c55e",
    warning:      "#f59e0b",
    info:         "#e50914",
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
    error:        "#dc2626",
    danger:       "#dc2626",
    dangerHover:  "#b91c1c",
    onAccent:     "#ffffff",
    overlay:      "rgba(0,0,0,0.4)",
    divider:      "rgba(0,0,0,0.08)",
    shadow:       "#000",
    success:      "#22c55e",
    warning:      "#f59e0b",
    info:         "#3b82f6",
  },
  radii:   { sm: 6, md: 12, lg: 20 },
  spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
};

export const themes = {
  default:        defaultTheme,
  amoled:         amoledTheme,
  "dark-red":     darkRedTheme,
  "light-default": lightDefaultTheme,
  "light-blue":    lightBlueTheme,
} as const;

export type ThemeName = keyof typeof themes;
