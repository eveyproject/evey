// The subtitle appearance every client offers: the model, the swatches and
// the two conversions. Platform-free on purpose - only WHERE the choice is
// stored differs (the keystore on a phone or an Android television,
// localStorage in a browser).

export interface SubtitleStyle {
  fontSize: number; // percent, 50-200 - same range as the web's slider
  color: string;
  bgColor: string;
  bgOpacity: number; // percent, 0-100
  position: "bottom" | "top";
  shadow: boolean;
}

export const DEFAULT_SUBTITLE_STYLE: SubtitleStyle = {
  fontSize: 100,
  color: "#ffffff",
  bgColor: "#000000",
  bgOpacity: 75,
  position: "bottom",
  shadow: false,
};

export const SUBTITLE_TEXT_COLORS = [
  { labelKey: "player.colors.white", value: "#ffffff" },
  { labelKey: "player.colors.yellow", value: "#fde047" },
  { labelKey: "player.colors.green", value: "#4ade80" },
  { labelKey: "player.colors.cyan", value: "#22d3ee" },
  { labelKey: "player.colors.orange", value: "#fb923c" },
] as const;

export interface SubtitleBgSwatch {
  labelKey: string;
  value: string;
  opacity?: number;
}

export const SUBTITLE_BG_COLORS: SubtitleBgSwatch[] = [
  { labelKey: "player.bgColors.black", value: "#000000" },
  { labelKey: "player.bgColors.darkGrey", value: "#1a1a1a" },
  { labelKey: "player.bgColors.navy", value: "#1e3a5f" },
  { labelKey: "player.bgColors.none", value: "#000000", opacity: 0 },
];

// Native cue text is ~5% of the video viewport height, mirroring the web's
// cueFontSizePx (useSubtitleManager.ts) so a 100% size preference reads about
// the same on both clients.
const CUE_HEIGHT_RATIO = 0.05;
const MIN_CUE_FONT_PX = 13;

export function subtitleCueFontSizePx(containerHeightPx: number, fontSizePct: number): number {
  if (containerHeightPx <= 0) return 0;
  return Math.max(MIN_CUE_FONT_PX, containerHeightPx * CUE_HEIGHT_RATIO * (fontSizePct / 100));
}

// subtitleBgRgba converts the hex swatch + opacity percent into the rgba()
// string React Native's backgroundColor accepts directly - mirrors the web's
// ::cue background rule (useSubtitleManager.ts). Falls back to black on an
// unexpected color value instead of producing NaN channels.
export function subtitleBgRgba(bgColor: string, bgOpacityPct: number): string {
  const hex = /^#[0-9a-fA-F]{6}$/.test(bgColor) ? bgColor : "#000000";
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  const a = Math.max(0, Math.min(100, bgOpacityPct)) / 100;
  return `rgba(${r},${g},${b},${a})`;
}
