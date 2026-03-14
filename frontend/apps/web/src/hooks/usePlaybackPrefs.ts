const AUDIO_KEY = "evey-pref-audio-lang";
const SUBTITLE_KEY = "evey-pref-subtitle-lang";

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
