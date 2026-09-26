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

// The profile-language pick is the one every player uses (@evey/api).
export { pickPreferredStreamIndex } from "@evey/api";
