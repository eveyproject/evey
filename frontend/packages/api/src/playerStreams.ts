import type { V1MediaStream } from "./generated/library.schemas";
import { isBurnInSubtitle, isImageSubtitle } from "./imageSubtitles";
import { pickPreferredStreamIndex } from "./langCodes";

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
  /** Director/cast commentary: never the automatic pick while a main track exists. */
  isCommentary?: boolean;
  isHearingImpaired?: boolean;
  /** Audio description for the visually impaired. */
  isVisualImpaired?: boolean;
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
      isCommentary: s.isCommentary,
      isHearingImpaired: s.isHearingImpaired,
      isVisualImpaired: s.isVisualImpaired,
    }));
}

/**
 * The audio tracks an automatic pick may land on: every non-commentary track,
 * or all of them when every track is commentary (commentary beats silence).
 * The server's default selection applies the same filter before any other
 * rule, so a client that picks its own default names the track the server
 * would serve.
 */
export function defaultAudioCandidates<T extends { isCommentary?: boolean }>(streams: T[]): T[] {
  const main = streams.filter((s) => !s.isCommentary);
  return main.length > 0 ? main : streams;
}

/**
 * The absolute index of the default audio track: the profile's languages in
 * order, then the file's default flag, then the first track - each step among
 * defaultAudioCandidates. Null when the title has no audio.
 */
export function pickDefaultAudioIndex(
  streams: { index: number; language?: string; isDefault?: boolean; isCommentary?: boolean }[],
  preferredLangs: string[],
): number | null {
  const candidates = defaultAudioCandidates(streams);
  if (candidates.length === 0) return null;
  return pickPreferredStreamIndex(candidates, preferredLangs)
    ?? candidates.find((s) => s.isDefault)?.index
    ?? candidates[0].index;
}

/**
 * The profile's "subtitle burn-in" choice, as stored under subtitle_burn_in:
 * "auto" burns only a bitmap the player cannot draw, "always" burns every
 * bitmap track (PGS included, at the cost of the copy rung), "never" burns
 * nothing, so a bitmap the player cannot draw stays off the picture rather
 * than turning the stream into a transcode. Text is never burnt: the server
 * only burns bitmaps, so a text track is drawn by the player in every mode.
 */
export type SubtitleBurnMode = "auto" | "always" | "never";

/** Reads subtitle_burn_in off a profile's preferences; anything else is auto. */
export function subtitleBurnMode(prefs: Record<string, string> | undefined): SubtitleBurnMode {
  const v = (prefs?.["subtitle_burn_in"] ?? "").trim().toLowerCase();
  return v === "always" || v === "never" ? v : "auto";
}

function burnsIn(codec: string | null | undefined, mode: SubtitleBurnMode): boolean {
  if (mode === "never") return false;
  if (mode === "always") return isImageSubtitle(codec);
  return isBurnInSubtitle(codec);
}

/** The subtitle tracks of a title, in file order, flagged for burn-in by the profile's mode. */
export function playerSubtitleStreams(streams: V1MediaStream[] | undefined, burnMode: SubtitleBurnMode = "auto"): PlayerSubtitleStream[] {
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
      burnIn: burnsIn(s.codec, burnMode),
    }));
}
