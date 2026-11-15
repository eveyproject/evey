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

// Where the appearance lives on the profile, as JSON, so it follows the
// viewer from one client to the next. Each client also keeps its last value
// locally: that is what an offline start, or a profile that never chose, uses.
export const SUBTITLE_STYLE_PREF_KEY = "subtitle_style";

const HEX_COLOR = /^#[0-9a-fA-F]{6}$/;

function clampPct(value: unknown, min: number, max: number, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback;
}

/**
 * Reads the appearance stored on a profile. null when the profile has none or
 * the value is not JSON; a field out of range falls back to its default, since
 * the value was written by another client, possibly an older or newer one.
 */
export function parseSubtitleStylePref(prefs: Record<string, string> | undefined): SubtitleStyle | null {
  const raw = prefs?.[SUBTITLE_STYLE_PREF_KEY];
  if (!raw) return null;
  let v: Partial<Record<keyof SubtitleStyle, unknown>>;
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return null;
    v = parsed as Partial<Record<keyof SubtitleStyle, unknown>>;
  } catch {
    return null;
  }
  const d = DEFAULT_SUBTITLE_STYLE;
  return {
    fontSize: clampPct(v.fontSize, 50, 200, d.fontSize),
    color: typeof v.color === "string" && HEX_COLOR.test(v.color) ? v.color : d.color,
    bgColor: typeof v.bgColor === "string" && HEX_COLOR.test(v.bgColor) ? v.bgColor : d.bgColor,
    bgOpacity: clampPct(v.bgOpacity, 0, 100, d.bgOpacity),
    position: v.position === "top" ? "top" : "bottom",
    shadow: v.shadow === true,
  };
}

/** The preferences map to send when saving the appearance (whole map, as for the theme). */
export function subtitleStylePreferences(
  current: Record<string, string> | undefined,
  style: SubtitleStyle,
): Record<string, string> {
  return { ...current, [SUBTITLE_STYLE_PREF_KEY]: JSON.stringify(style) };
}

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
