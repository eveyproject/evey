import type { ThemeTokens } from "@evey/theme";

// A categorical chart palette (pie/bar series) is allowed to exist - series
// must stay visually distinct - but it must not be duplicated across screens
// as raw hex, and it must render legibly on both dark AND light themes (two
// of the five shipped themes are light). The first slot reuses the theme's
// own accent so charts feel on-brand; the rest are fixed, medium-saturation
// hues chosen to keep contrast against both near-white and near-black
// surfaces, since gifted-charts fills are painted directly (no CSS var
// support), so they can't be derived from the theme the way flat UI colors
// (text/border/surface) are.
const FIXED_HUES = [
  "#f59e0b", // amber
  "#10b981", // emerald
  "#ef4444", // red
  "#8b5cf6", // violet
  "#ec4899", // pink
  "#14b8a6", // teal
  "#f97316", // orange
] as const;

/**
 * Returns a stable, theme-aware categorical palette for chart series
 * (genre breakdown pie, etc). Same index always yields the same color for a
 * given theme, so re-renders don't reshuffle which series has which color.
 */
export function getChartPalette(theme: ThemeTokens): string[] {
  return [theme.colors.accent, ...FIXED_HUES];
}

export function chartColorAt(theme: ThemeTokens, index: number): string {
  const palette = getChartPalette(theme);
  return palette[index % palette.length];
}
