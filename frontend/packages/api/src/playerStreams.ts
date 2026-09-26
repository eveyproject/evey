import type { V1MediaStream } from "./generated/library.schemas";
import { isBurnInSubtitle } from "./imageSubtitles";

/** An audio track as every player lists it. */
export interface PlayerAudioStream {
  /** Absolute stream index in the file (what the server's track pins name). */
  index: number;
  /** 0-based among the audio streams (ffmpeg -map 0:a:N). */
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  /** Source codec (ac3/eac3/aac...): drives the web's wasm-audio path. */
  codec?: string;
  channels?: number;
}

/** A subtitle track as every player lists it. */
export interface PlayerSubtitleStream {
  index: number;
  /** 0-based among the subtitle streams (ffmpeg -map 0:s:N). */
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  /** Forced track: shown on its own when the audio matches its language. */
  isForced?: boolean;
  isExternal?: boolean;
  externalPath?: string;
  codec?: string;
  /**
   * ONLY "the server must burn this in": a bitmap no player can draw. PGS is a
   * bitmap too, but the server decodes it once and the player draws it, so it
   * is NOT a burn-in - flagging it sent ?subtitleTrack= and turned a
   * direct-playing remux into a full transcode.
   */
  burnIn?: boolean;
}

/** The audio tracks of a title, in file order. */
export function playerAudioStreams(streams: V1MediaStream[] | undefined): PlayerAudioStream[] {
  return (streams ?? [])
    .filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
    .map((s, i) => ({
      index: s.index ?? 0,
      trackIndex: i,
      language: s.language,
      title: s.title,
      isDefault: s.isDefault,
      codec: s.codec,
      channels: s.channels,
    }));
}

/** The subtitle tracks of a title, in file order. */
export function playerSubtitleStreams(streams: V1MediaStream[] | undefined): PlayerSubtitleStream[] {
  return (streams ?? [])
    .filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
    .map((s, i) => ({
      index: s.index ?? 0,
      trackIndex: i,
      language: s.language,
      title: s.title,
      isDefault: s.isDefault,
      isForced: s.isForced,
      isExternal: s.isExternal,
      externalPath: s.externalPath,
      codec: s.codec,
      burnIn: isBurnInSubtitle(s.codec),
    }));
}
