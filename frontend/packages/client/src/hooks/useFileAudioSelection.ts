import { useEffect, useState } from "react";
import { normalizeLangCode, type PlayerAudioStream } from "@evey/api";

export interface NativeAudioTrack {
  id?: string;
  language?: string;
  label?: string;
  /** The track's own title (Matroska track name), when the file has one. */
  name?: string;
}

/**
 * The native track that plays the given stream, or null when nothing lines up.
 * Matched on what the track IS - its language, then its title - and never on
 * position alone: the player lists its tracks renderer by renderer, so an
 * E-AC-3 track the ac3go renderer takes comes before an AAC one even when the
 * file stores it second, and position 1 then names the other language.
 * Position is only trusted for tracks that carry no language to check.
 */
export function pickNativeAudioTrack(
  nativeTracks: NativeAudioTrack[],
  stream: PlayerAudioStream,
  streams: PlayerAudioStream[],
): NativeAudioTrack | null {
  if (!nativeTracks || nativeTracks.length === 0) return null;
  const want = normalizeLangCode(stream.language);
  const sameLang = want ? nativeTracks.filter((t) => normalizeLangCode(t.language) === want) : [];
  if (sameLang.length === 1) return sameLang[0];
  const pool = sameLang.length > 1 ? sameLang : nativeTracks;
  if (stream.title) {
    const byName = pool.filter((t) => t.name === stream.title);
    if (byName.length === 1) return byName[0];
  }
  if (sameLang.length > 1) {
    // Several tracks of one language and no title to tell them apart: the
    // player keeps their relative order, so the k-th of the file is its k-th.
    const peers = streams.filter((s) => normalizeLangCode(s.language) === want);
    const k = peers.findIndex((s) => s.index === stream.index);
    return peers.length === sameLang.length && k >= 0 ? sameLang[k] : null;
  }
  const atPosition = nativeTracks[stream.trackIndex];
  if (atPosition && !normalizeLangCode(atPosition.language)) return atPosition;
  return null;
}

/**
 * The native track to switch to so the player plays the given stream, or null
 * when it already does or nothing lines up.
 */
export function nativeTrackToApply(
  nativeTracks: NativeAudioTrack[],
  current: NativeAudioTrack | null | undefined,
  stream: PlayerAudioStream,
  streams: PlayerAudioStream[],
): NativeAudioTrack | null {
  const want = pickNativeAudioTrack(nativeTracks, stream, streams);
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
 * a change made in the player all stayed on screen and out of the speakers.
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
      const track = nativeTrackToApply(nativeTracks, player.audioTrack, stream, audioStreams);
      if (track) player.audioTrack = track;
    } catch { /* released */ }
  }, [player, enabled, nativeTracks, audioStreams, selectedAudioIndex]);
}
