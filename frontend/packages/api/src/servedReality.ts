import type { V1PlayedState, V1ServedLeg } from "./generated/playback.schemas";
import type { PlayMethodKind } from "./playMethod";

// What a playback IS, for every surface that shows it (the player's stream
// panel, the admin session card on the web and on the phone). Two witnesses
// only, never a verdict:
//   - served: what the server sent, observed on the responses that carried the
//     bytes (engine, copy or encode, source track, codec read from the init);
//   - played: what the player reports it decodes.
// When the two disagree the view says so instead of picking one.

export interface ServedView {
  copy: boolean;
  engine: string; // mkvgo | ffmpeg | file
  backend: string; // video encode only: software | qsv | vaapi | nvenc | rkmpp
  codec: string; // short name (h264, aac, mp3...); "" when the bytes carry every track
  sourceCodec: string; // the codec the bytes were encoded from, "" on a copy
  track: number; // audio: source track (0-based); -1 = every track / unknown
  height: number;
  channels: number;
}

export interface PlayedView {
  source: "hls" | "file" | "";
  videoCodec: string;
  height: number;
  audioTrack: number; // -1 = unknown
  audioCodec: string;
  decoder: "native" | "wasm" | "";
}

export interface SessionReality {
  method: PlayMethodKind | "";
  video?: ServedView;
  audio?: ServedView;
  played?: PlayedView;
  // The player says it plays another track than the one the server sends.
  trackMismatch?: { played: number; served: number };
  // The player's audio decoder was set up for another codec than the one sent.
  codecMismatch?: { played: string; served: string };
}

// codecName turns the vocabulary each witness speaks into one short name: the
// server reads ffprobe names off the init ("aac", "mp3", "hevc"), a browser
// reports RFC 6381 strings off its SourceBuffer ("mp4a.40.2", "avc1.640028").
export function codecName(raw?: string): string {
  const c = (raw ?? "").trim().toLowerCase();
  if (!c) return "";
  if (c.startsWith("avc1") || c.startsWith("avc3") || c === "h264") return "h264";
  if (c.startsWith("hvc1") || c.startsWith("hev1") || c === "hevc" || c === "h265") return "hevc";
  if (c.startsWith("av01") || c === "av1") return "av1";
  if (c.startsWith("vp09") || c === "vp9") return "vp9";
  if (c === "mp4a.40.34" || c === "mp4a.6b" || c === "mp4a.69" || c === "mp3" || c === "a_mpeg/l3") return "mp3";
  if (c.startsWith("mp4a") || c === "aac") return "aac";
  if (c === "ac-3" || c === "ac3") return "ac3";
  if (c === "ec-3" || c === "eac3" || c === "e-ac-3") return "eac3";
  if (c === "opus") return "opus";
  if (c === "flac") return "flac";
  return c;
}

function servedView(l?: V1ServedLeg): ServedView | undefined {
  if (!l || !l.engine) return undefined;
  const copy = l.copy ?? false;
  const codec = codecName(l.codec);
  const source = codecName(l.sourceCodec);
  return {
    copy,
    engine: l.engine,
    backend: copy ? "" : (l.backend ?? ""),
    codec,
    sourceCodec: !copy && source && source !== codec ? source : "",
    // proto3 JSON leaves a zero out: an absent track is track 0, -1 is sent.
    track: l.track ?? 0,
    height: l.height ?? 0,
    channels: l.channels ?? 0,
  };
}

function playedView(p?: V1PlayedState): PlayedView | undefined {
  if (!p || !p.source) return undefined;
  const source = p.source === "hls" || p.source === "file" ? p.source : "";
  const decoder = p.audioDecoder === "native" || p.audioDecoder === "wasm" ? p.audioDecoder : "";
  return {
    source,
    videoCodec: codecName(p.videoCodec),
    height: p.height ?? 0,
    audioTrack: p.audioTrack ?? 0,
    audioCodec: codecName(p.audioCodec),
    decoder,
  };
}

// The same rule as the server's Served.Method: the file itself is a direct
// play, copied samples a remux, any encoded leg a transcode.
export function servedMethod(video?: ServedView, audio?: ServedView): PlayMethodKind | "" {
  const legs = [video, audio].filter((l): l is ServedView => !!l);
  if (legs.length === 0) return "";
  if (legs.some((l) => l.engine === "file")) return "direct_play";
  return legs.some((l) => !l.copy) ? "transcode" : "direct_stream";
}

export function describeSession(input: { servedVideo?: V1ServedLeg; servedAudio?: V1ServedLeg; played?: V1PlayedState }): SessionReality {
  const video = servedView(input.servedVideo);
  const audio = servedView(input.servedAudio);
  const played = playedView(input.played);
  const out: SessionReality = { method: servedMethod(video, audio), video, audio, played };
  if (audio && played) {
    if (audio.track >= 0 && played.audioTrack >= 0 && audio.track !== played.audioTrack) {
      out.trackMismatch = { played: played.audioTrack, served: audio.track };
    }
    if (audio.codec && played.audioCodec && audio.codec !== played.audioCodec) {
      out.codecMismatch = { played: played.audioCodec, served: audio.codec };
    }
  }
  return out;
}
