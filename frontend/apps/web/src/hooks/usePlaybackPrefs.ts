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
  } catch (err) { console.warn("localStorage write failed", err); }
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

const VOLUME_BOOST_KEY = "evey:volume-boost";

// Amplification factor for the >100% zone (1..2). Clamped and NaN-guarded so a
// corrupted value can never feed a bogus gain into the audio graph.
export function getVolumeBoost(): number {
  const raw = get(VOLUME_BOOST_KEY);
  if (!raw) return 1;
  const n = Number(raw);
  if (!isFinite(n)) return 1;
  return Math.min(2, Math.max(1, n));
}

export function setVolumeBoost(boost: number) {
  set(VOLUME_BOOST_KEY, boost > 1 ? String(boost) : null);
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
