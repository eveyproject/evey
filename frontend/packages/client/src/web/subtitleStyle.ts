import { DEFAULT_SUBTITLE_STYLE, type SubtitleStyle } from "../lib/subtitleStyle";

// The model lives in ../lib/subtitleStyle; this file only keeps the choice,
// in the browser's own storage.
export * from "../lib/subtitleStyle";

const STYLE_KEY = "evey-subtitle-style";

/** null when nothing is stored, storage is refused, or the value is corrupt. */
export function getSubtitleStylePref(): SubtitleStyle | null {
  try {
    const raw = localStorage.getItem(STYLE_KEY);
    if (!raw) return null;
    return { ...DEFAULT_SUBTITLE_STYLE, ...(JSON.parse(raw) as Partial<SubtitleStyle>) };
  } catch {
    return null;
  }
}

export function saveSubtitleStylePref(style: SubtitleStyle): void {
  try {
    localStorage.setItem(STYLE_KEY, JSON.stringify(style));
  } catch {
    /* private window or quota: the preference just won't stick */
  }
}
