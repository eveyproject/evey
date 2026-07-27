// Maps the audio track the user picked (streamInfo order) to the track the
// NATIVE player exposes from the HLS manifest, so switching audio is an instant
// client-side select (player.audioTrack = ...) with no reload - decoupled from
// the manifest DEFAULT flag. The native tracks and the streamInfo audio streams
// are both in source order, so position is the reliable key; language is only a
// fallback when the counts don't line up.

import { normalizeLangCode } from "./langCodes";

export interface NativeAudioTrack {
  id?: string;
  language?: string;
  label?: string;
}

/**
 * Returns the native track for the given 0-based audio position, or the first
 * track whose language matches, or null when nothing lines up (manifest not
 * parsed yet, or a genuine mismatch - the caller then falls back to a restart).
 */
export function pickNativeAudioTrack(
  nativeTracks: NativeAudioTrack[],
  trackIndex: number,
  language: string | undefined,
): NativeAudioTrack | null {
  if (!nativeTracks || nativeTracks.length === 0) return null;
  if (trackIndex >= 0 && trackIndex < nativeTracks.length) return nativeTracks[trackIndex];
  const want = normalizeLangCode(language);
  if (want) {
    const byLang = nativeTracks.find((t) => normalizeLangCode(t.language) === want);
    if (byLang) return byLang;
  }
  return null;
}

// pickPreferredStreamIndex resolves the stream `index` (the manifest-relative
// id used by streamInfo / restartWithTracks - NOT the 0-based trackIndex)
// whose language matches the first entry of an ordered preference list
// that's actually present in the file. Used to apply a profile's
// audio_languages/subtitle_languages preference (see player/[id].tsx) - a
// scoped port of the web's resolveDefaultAudio/resolveDefaultSubtitle
// preference step (hlsPlayerUtils.ts); mobile has no per-series or
// device-local override to reconcile, so only the profile-preference step
// is needed here.
export function pickPreferredStreamIndex(
  streams: { index: number; language?: string }[],
  preferredLangs: string[],
): number | null {
  for (const lang of preferredLangs) {
    if (!lang) continue;
    const want = normalizeLangCode(lang);
    if (!want) continue;
    const match = streams.find((s) => normalizeLangCode(s.language) === want);
    if (match) return match.index;
  }
  return null;
}
