// Maps the audio track the user picked (streamInfo order) to the track the
// NATIVE player exposes from the HLS manifest, so switching audio is an instant
// client-side select (player.audioTrack = ...) with no reload - decoupled from
// the manifest DEFAULT flag. The native tracks and the streamInfo audio streams
// are both in source order, so position is the reliable key; language is only a
// fallback when the counts don't line up.

export interface NativeAudioTrack {
  id?: string;
  language?: string;
  label?: string;
}

function normLang(l: string | undefined): string {
  // ISO 639-2 ("fre") vs 639-1 ("fr"): compare the first two letters, which
  // agree for every language pair we ship. Lowercased, trimmed.
  return (l ?? "").trim().toLowerCase().slice(0, 2);
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
  const want = normLang(language);
  if (want) {
    const byLang = nativeTracks.find((t) => normLang(t.language) === want);
    if (byLang) return byLang;
  }
  return null;
}
