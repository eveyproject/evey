import { useCallback, useEffect, useRef, useState } from "react";
import type { PlayerAudioStream, V1PlayedState } from "@evey/api";
import {
  muxedAudioCodec,
  nativeTrackForOrdinal,
  parseMasterAudioRenditions,
  renditionOfNativeTrack,
  renditionOrdinal,
  type MasterAudioRendition,
} from "../lib/masterRenditions";
import { masterTrackOrdinal } from "../lib/playedState";

// The native player's witness of a session (the web's is playedState): what
// it decodes, read from what it loaded. The master it plays is read once more
// so every audio rendition is tied to the source track it carries; the native
// audio track then names the source track by the rendition it is, never by
// its language or its place in a list.

export interface NativeAudioTrackLike {
  id?: string;
  name?: string;
  language?: string;
}

export interface NativePlayerLike {
  audioTrack?: NativeAudioTrackLike | null;
  availableAudioTracks?: NativeAudioTrackLike[];
  videoTrack?: { size?: { width?: number; height?: number } | null; mimeType?: string | null } | null;
}

interface MasterFacts {
  url: string;
  renditions: MasterAudioRendition[];
  muxedCodec?: string;
}

export function isSegmentedUrl(url: string | null | undefined): boolean {
  return !!url && url.includes("/hls/");
}

// nativePlayed is the report itself, pure: what the player has loaded and
// what the master says about it.
export function nativePlayed(
  player: NativePlayerLike | null,
  streamUrl: string | null | undefined,
  master: MasterFacts | null,
  audioStreams: PlayerAudioStream[],
): V1PlayedState | undefined {
  if (!streamUrl) return undefined;
  let height: number | undefined;
  let videoCodec: string | undefined;
  let track: NativeAudioTrackLike | null | undefined;
  try {
    height = player?.videoTrack?.size?.height || undefined;
    videoCodec = player?.videoTrack?.mimeType || undefined;
    track = player?.audioTrack;
  } catch {
    // A released player: nothing to read.
  }
  if (!isSegmentedUrl(streamUrl)) {
    // The file: Media3 names a file's track by the number the file gives it.
    const id = Number(track?.id);
    const s = Number.isInteger(id) ? audioStreams.find((a) => a.sourceTrackId === id) : undefined;
    return { source: "file", audioTrack: s ? s.trackIndex : -1, height, videoCodec };
  }
  const facts = master && master.url === streamUrl ? master : null;
  if (facts && facts.renditions.length > 0) {
    const r = renditionOfNativeTrack(facts.renditions, track);
    return { source: "hls", height, videoCodec, audioTrack: renditionOrdinal(r, audioStreams), audioCodec: r?.codec, audioDecoder: "native" };
  }
  // A master with its audio muxed carries the track it was asked for. Until
  // the master is read, which kind it is is not known: no track is named.
  return {
    source: "hls",
    height,
    videoCodec,
    audioTrack: facts ? masterTrackOrdinal(streamUrl) : -1,
    audioCodec: facts?.muxedCodec,
    audioDecoder: facts?.muxedCodec ? "native" : undefined,
  };
}

export function useNativePlayed(player: NativePlayerLike | null, streamUrl: string | null | undefined, audioStreams: PlayerAudioStream[]) {
  const [master, setMaster] = useState<MasterFacts | null>(null);
  useEffect(() => {
    setMaster(null);
    if (!streamUrl || !isSegmentedUrl(streamUrl)) return;
    let gone = false;
    fetch(streamUrl)
      .then((r) => (r.ok ? r.text() : ""))
      .then((text) => {
        if (gone || !text) return;
        setMaster({ url: streamUrl, renditions: parseMasterAudioRenditions(text), muxedCodec: muxedAudioCodec(text) });
      })
      .catch(() => { /* no master facts: the report names what it can */ });
    return () => { gone = true; };
  }, [streamUrl]);

  const live = useRef({ player, streamUrl, master, audioStreams });
  live.current = { player, streamUrl, master, audioStreams };

  const getPlayed = useCallback((): V1PlayedState | undefined => {
    const l = live.current;
    return nativePlayed(l.player, l.streamUrl, l.master, l.audioStreams);
  }, []);

  // The native track that carries source audio track `ordinal`, in whatever
  // audio group: undefined when the master does not name one (the caller
  // then falls back to its own pick).
  const trackFor = useCallback((ordinal: number): NativeAudioTrackLike | undefined => {
    const l = live.current;
    if (!l.master || l.master.url !== l.streamUrl) return undefined;
    let tracks: NativeAudioTrackLike[] = [];
    try { tracks = l.player?.availableAudioTracks ?? []; } catch { /* released */ }
    return nativeTrackForOrdinal(tracks, l.master.renditions, ordinal, l.audioStreams);
  }, []);

  // The master of the stream playing has been read: trackFor can answer.
  const ready = !!master && master.url === streamUrl;
  return { getPlayed, trackFor, ready };
}
