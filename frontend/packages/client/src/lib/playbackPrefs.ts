// The playback preferences every client writes onto the profile: the option
// lists, the legacy code migration and the reader. Shared so the browser
// television offers exactly what the Android one does.

export type PrefMap = Record<string, string>;

// ISO 639-1 (2-letter) to match the audio_languages/subtitle_languages
// convention shared with web (LanguageSettings.tsx) and mobile (settings/
// playback.tsx) - the preference is stored per-profile and read by every
// client's track resolution, so all three must agree on the same codes.
// Language names are each language's own endonym (not translated) - same
// choice web/mobile made, and consistent with the untranslated codec proper
// nouns (H.264, AC3, ...) below. "None" reuses web's common.noPreference key.
export const LANG_OPTIONS = [
  { value: "", labelKey: "common.noPreference", fallback: "No preference" },
  { value: "en", labelKey: "", fallback: "English" },
  { value: "fr", labelKey: "", fallback: "Français" },
  { value: "es", labelKey: "", fallback: "Español" },
  { value: "de", labelKey: "", fallback: "Deutsch" },
  { value: "it", labelKey: "", fallback: "Italiano" },
  { value: "pt", labelKey: "", fallback: "Português" },
  { value: "ja", labelKey: "", fallback: "日本語" },
  { value: "ko", labelKey: "", fallback: "한국어" },
  { value: "zh", labelKey: "", fallback: "中文" },
  { value: "ru", labelKey: "", fallback: "Русский" },
  { value: "ar", labelKey: "", fallback: "العربية" },
];

// This screen used to save ISO 639-2/B codes ("fre", "ger", ...) under these
// same preference keys. Map them to 639-1 on read so a value saved before
// this fix still highlights the matching option instead of showing none
// selected, until the user re-saves under the new 2-letter code.
const LEGACY_LANG_MAP: Record<string, string> = {
  eng: "en", fre: "fr", spa: "es", ger: "de", ita: "it",
  por: "pt", jpn: "ja", kor: "ko", chi: "zh", rus: "ru", ara: "ar",
};

function legacyLangToIso1(code: string | undefined): string | undefined {
  if (code === undefined) return undefined;
  return LEGACY_LANG_MAP[code] ?? code;
}

// Resolves one language preference for this profile. audio_languages/
// subtitle_languages is the shared web/mobile/tv convention (CSV of ISO 639-1
// codes, first entry = highest priority); this screen only ever writes a
// single code, so we read just the first entry. Falls back to the TV-only
// default_audio_language/default_subtitle_language key (soft migration on
// read) so a language picked before this fix stays selected until re-saved.
export function resolveLangPref(
  prefs: PrefMap,
  sharedKey: "audio_languages" | "subtitle_languages",
  legacyKey: "default_audio_language" | "default_subtitle_language",
): string {
  const raw = prefs[sharedKey] ?? legacyLangToIso1(prefs[legacyKey]);
  if (raw === undefined) return "";
  return raw.split(",")[0]?.trim() ?? "";
}

export const BITRATE_OPTIONS = [
  { value: "0", labelKey: "auth.settings.playback.bitrateUnlimited", fallback: "Unlimited" },
  { value: "2000", labelKey: "", fallback: "2 Mbps" },
  { value: "4000", labelKey: "", fallback: "4 Mbps" },
  { value: "8000", labelKey: "", fallback: "8 Mbps" },
  { value: "10000", labelKey: "", fallback: "10 Mbps" },
  { value: "15000", labelKey: "", fallback: "15 Mbps" },
  { value: "20000", labelKey: "", fallback: "20 Mbps" },
  { value: "40000", labelKey: "", fallback: "40 Mbps" },
];

export const VIDEO_CODEC_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "h264", labelKey: "", fallback: "H.264" },
  { value: "hevc", labelKey: "", fallback: "HEVC (H.265)" },
  { value: "av1", labelKey: "", fallback: "AV1" },
];

export const AUDIO_CODEC_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "aac", labelKey: "", fallback: "AAC" },
  { value: "ac3", labelKey: "", fallback: "AC3" },
  { value: "eac3", labelKey: "", fallback: "E-AC3" },
  { value: "opus", labelKey: "", fallback: "Opus" },
];

export const AUDIO_CHANNELS_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "2", labelKey: "auth.settings.playback.channelsStereo", fallback: "Stereo (2.0)" },
  { value: "6", labelKey: "auth.settings.playback.channels51", fallback: "Surround (5.1)" },
  { value: "8", labelKey: "auth.settings.playback.channels71", fallback: "Surround (7.1)" },
];

export const SUBTITLE_BURN_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.subtitleBurnAuto", fallback: "Auto" },
  { value: "always", labelKey: "auth.settings.playback.subtitleBurnAlways", fallback: "Always burn in" },
  { value: "never", labelKey: "auth.settings.playback.subtitleBurnNever", fallback: "Never burn in" },
];

// Boolean toggles read as a 2-row choice ("Enabled"/"Disabled") rather than a
// react-native Switch: D-pad focus/select on a row is the idiom the rest of
// this screen (and the tabs settings screen) already uses for every choice.
export const TOGGLE_OPTIONS = [
  { value: true, labelKey: "common.enabled", fallback: "Enabled" },
  { value: false, labelKey: "common.disabled", fallback: "Disabled" },
];
