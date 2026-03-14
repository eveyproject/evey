const AUDIO_KEY = "evey-pref-audio-lang";
const SUBTITLE_KEY = "evey-pref-subtitle-lang";
const SERIES_PREFS_PREFIX = "evey:prefs:";
const SUBTITLE_STYLE_KEY = "evey:subtitle-style";

function get(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}

function set(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {}
}

export function getPreferredAudioLang(): string | null {
  return get(AUDIO_KEY);
}

export function setPreferredAudioLang(lang: string | null) {
  set(AUDIO_KEY, lang);
}

export function getPreferredSubtitleLang(): string | null {
  return get(SUBTITLE_KEY);
}

export function setPreferredSubtitleLang(lang: string | null) {
  set(SUBTITLE_KEY, lang);
}

interface SeriesTrackPrefs {
  audioLang?: string;
  subtitleLang?: string | null;
}

export function getSeriesTrackPrefs(seriesId: string): SeriesTrackPrefs | null {
  const raw = get(SERIES_PREFS_PREFIX + seriesId);
  if (!raw) return null;
  try { return JSON.parse(raw) as SeriesTrackPrefs; } catch { return null; }
}

export function setSeriesTrackPrefs(seriesId: string, prefs: SeriesTrackPrefs) {
  set(SERIES_PREFS_PREFIX + seriesId, JSON.stringify(prefs));
}

export interface SubtitleStyle {
  fontSize: number;
  color: string;
  fontFamily: string;
  bgColor: string;
  bgOpacity: number;
  position: "bottom" | "top";
  shadow: boolean;
}

export const DEFAULT_SUBTITLE_STYLE: SubtitleStyle = {
  fontSize: 100,
  color: "#ffffff",
  fontFamily: "",
  bgColor: "#000000",
  bgOpacity: 75,
  position: "bottom",
  shadow: false,
};

export function getSubtitleStyle(): SubtitleStyle {
  const raw = get(SUBTITLE_STYLE_KEY);
  if (!raw) return { ...DEFAULT_SUBTITLE_STYLE };
  try { return { ...DEFAULT_SUBTITLE_STYLE, ...(JSON.parse(raw) as Partial<SubtitleStyle>) }; }
  catch { return { ...DEFAULT_SUBTITLE_STYLE }; }
}

export function setSubtitleStyle(style: SubtitleStyle) {
  set(SUBTITLE_STYLE_KEY, JSON.stringify(style));
}
