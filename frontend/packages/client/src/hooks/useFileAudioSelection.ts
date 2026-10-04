import { useEffect, useState } from "react";
import { normalizeLangCode, type PlayerAudioStream } from "@evey/api";

export interface NativeAudioTrack {
  id?: string;
  language?: string;
  label?: string;
}

/**
 * The native track for a 0-based audio position, or the first track whose
 * language matches, or null when nothing lines up. The native tracks and the
 * file's audio streams are both in source order, so position is the reliable
 * key; language is only a fallback when the counts differ.
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

/**
 * The native track to switch to so the player plays the given stream, or null
 * when it already does or nothing lines up.
 */
export function nativeTrackToApply(
  nativeTracks: NativeAudioTrack[],
  current: NativeAudioTrack | null | undefined,
  trackIndex: number,
  language: string | undefined,
): NativeAudioTrack | null {
  const want = pickNativeAudioTrack(nativeTracks, trackIndex, language);
  if (!want) return null;
  if (current?.id !== undefined && current.id === want.id) return null;
  return want;
}

/** What the hook needs of expo-video's player. */
export interface AudioTrackPlayer {
  audioTrack?: NativeAudioTrack | null;
  availableAudioTracks?: NativeAudioTrack[];
  addListener: (event: "availableAudioTracksChange", cb: () => void) => { remove: () => void };
}

/**
 * Applies the audio selection to a player that plays the FILE itself (direct
 * play, downloaded copy). An HLS URL carries the selection; a file opens on
 * its own default track, so the title page's pick, the profile's language and
 * a change made in the player all stayed on screen and out of the speakers
 * (tablet, 2026-10-03).
 */
export function useFileAudioSelection(
  player: AudioTrackPlayer | null | undefined,
  { enabled, audioStreams, selectedAudioIndex }: { enabled: boolean; audioStreams: PlayerAudioStream[]; selectedAudioIndex: number },
): void {
  const [nativeTracks, setNativeTracks] = useState<NativeAudioTrack[]>([]);

  useEffect(() => {
    if (!player) return;
    const read = () => {
      try { setNativeTracks(player.availableAudioTracks ?? []); } catch { /* released */ }
    };
    read();
    const sub = player.addListener("availableAudioTracksChange", read);
    return () => sub.remove();
  }, [player]);

  useEffect(() => {
    if (!player || !enabled) return;
    const stream = audioStreams.find((s) => s.index === selectedAudioIndex);
    if (!stream) return;
    try {
      const track = nativeTrackToApply(nativeTracks, player.audioTrack, stream.trackIndex, stream.language);
      if (track) player.audioTrack = track;
    } catch { /* released */ }
  }, [player, enabled, nativeTracks, audioStreams, selectedAudioIndex]);
}
