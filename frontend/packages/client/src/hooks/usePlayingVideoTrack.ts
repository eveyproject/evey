import { useEffect, useState } from "react";
import { playingFromVideoTrack, type PlayingVariant } from "../lib/servedVariant";

type VideoTrackLike = Parameters<typeof playingFromVideoTrack>[0];

// Structural: an expo-video VideoPlayer, without importing expo-video here.
export interface VideoTrackSource {
  readonly videoTrack?: VideoTrackLike;
  readonly availableVideoTracks?: NonNullable<VideoTrackLike>[];
  // any: expo types it per event name, which no structural signature matches.
  addListener(event: any, listener: any): { remove(): void };
}

function bitratesOf(tracks: NonNullable<VideoTrackLike>[] | undefined): number[] {
  return (tracks ?? []).map((t) => t.peakBitrate ?? t.bitrate ?? t.averageBitrate ?? 0);
}

// The HLS variant the native player (ExoPlayer, AVPlayer) decodes right now and
// every variant's BANDWIDTH, followed through its own ABR switches.
export function usePlayingVideoTrack(player: VideoTrackSource | null | undefined): { playing: PlayingVariant | null; ladderBitrates: number[] } {
  const [state, setState] = useState<{ playing: PlayingVariant | null; ladderBitrates: number[] }>({ playing: null, ladderBitrates: [] });
  useEffect(() => {
    if (!player) return;
    const read = () => {
      // A released player throws on every property read.
      try {
        setState({ playing: playingFromVideoTrack(player.videoTrack), ladderBitrates: bitratesOf(player.availableVideoTracks) });
      } catch { /* released */ }
    };
    read();
    const trackSub = player.addListener("videoTrackChange", read);
    const loadSub = player.addListener("sourceLoad", read);
    return () => {
      trackSub.remove();
      loadSub.remove();
    };
  }, [player]);
  return state;
}
