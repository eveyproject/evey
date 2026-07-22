import type { V1MediaStream } from "@evey/api";
import { isWasmDecodableAudioCodec } from "@/utils/codec";

// Playback difficulty, derived from the server's OWN decision for THIS device.
//
// It is deliberately not a guess from the codec names: the server already
// decides copy vs encode per track in MakePlaybackDecisionWithPrefs, and
// GetStreamInfo hands us play_method + video_decision + audio_decision +
// transcode_reasons. Re-deriving that client-side would be a second, divergent
// implementation of the rule set - the thing that made "direct play" claims
// wrong in the past.
//
// Known blind spot, worth remembering before trusting this too far: the RPC
// reports the *decision*, not the *plan*. The resolver can still fall back to a
// full transcode for reasons the decision never sees - no keyframe index yet,
// VFR source, tight disk, no free GPU slot. So "direct" here means "nothing in
// the media forces a re-encode on this device", not "will certainly not
// transcode".
export type PlaybackLevel =
  | "direct"
  | "remux"
  | "wasmAudio"
  | "audioOnly"
  | "videoSd"
  | "video"
  | "heavy";

/** What it costs the server, which is what the badge actually announces. */
export type Difficulty = "easy" | "moderate" | "demanding" | "hard";

const DIFFICULTY_OF: Record<PlaybackLevel, Difficulty> = {
  direct: "easy",
  remux: "easy",
  // AC-3 copied out-of-band and decoded by the browser: no server encode at all.
  wasmAudio: "easy",
  audioOnly: "moderate",
  // Re-encoding SD is not the same job as re-encoding HD, and grading only on
  // "is the video re-encoded?" put an old 720x404 DivX in the same bucket as a
  // 1080p title. The server's real cost model (VideoCostModel.CostFor, keyed on
  // ladder class AND height, calibrated at boot) is not exposed on any endpoint,
  // so height is the stand-in - it is the dominant term in that model anyway.
  videoSd: "moderate",
  video: "demanding",
  heavy: "hard",
};

/** Above SD: re-encoding below this is cheap even on the weak-NAS target. */
const SD_MAX_HEIGHT = 576;

export interface PlaybackDifficulty {
  /** The operation, for the tooltip. */
  level: PlaybackLevel;
  /** The headline, which is what the viewer wants to know. */
  difficulty: Difficulty;
  /** Raw server reasons, for the tooltip. */
  reasons: string[];
}

// Above 1080p a software encode stops keeping up on the weak NAS this project
// targets, so this separates "demanding" from "hard".
const HEAVY_HEIGHT = 1440;

export function videoStream(streams?: V1MediaStream[]): V1MediaStream | undefined {
  return streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

// Mirrors internal/pkg/hdr DetectHDR, minus Dolby Vision: dovi_profile and
// dovi_bl_compat are not in the MediaStream proto, so a DoVi title reads as
// plain HDR10 here. The server still transcodes it (DoVi always tonemaps), and
// that shows up in transcode_reasons, so the BADGE stays right even though this
// label is coarse.
export function hdrLabel(stream?: V1MediaStream): string | null {
  if (!stream) return null;
  switch (stream.colorTransfer?.trim().toLowerCase()) {
    case "smpte2084":
      return "HDR10";
    case "arib-std-b67":
      return "HLG";
  }
  // Legacy rows probed before color_transfer was stored: BT.2020 + 10-bit is
  // HDR, but the two flavours can't be told apart without the transfer, so the
  // server reports hdr10 as the conservative default. Match it.
  const cs = stream.colorSpace?.trim().toLowerCase() ?? "";
  if (cs.startsWith("bt2020") && (stream.bitDepth ?? 0) >= 10) return "HDR10";
  return null;
}

export function resolutionLabel(stream?: V1MediaStream): string | null {
  const h = stream?.height ?? 0;
  if (h <= 0) return null;
  if (h >= 2160) return "4K";
  if (h >= 1440) return "1440p";
  if (h >= 1080) return "1080p";
  if (h >= 720) return "720p";
  return `${h}p`;
}

export function channelLabel(channels?: number): string | null {
  switch (channels) {
    case undefined:
    case 0:
      return null;
    case 1:
      return "Mono";
    case 2:
      return "2.0";
    case 6:
      return "5.1";
    case 8:
      return "7.1";
    default:
      return `${channels}ch`;
  }
}

function withDifficulty(level: PlaybackLevel, reasons: string[]): PlaybackDifficulty {
  return { level, difficulty: DIFFICULTY_OF[level], reasons };
}

/** The decision plus the two source facts the grading needs. A list response
    carries these flat (the bulk endpoint sends them); a detail response carries
    them inside `streams`. One rule set reads both. */
export interface PlaybackFacts {
  playMethod?: string;
  videoDecision?: string;
  audioDecision?: string;
  transcodeReasons?: string[];
  videoHeight?: number;
  audioCodec?: string;
}

export function classifyPlaybackFacts(
  info: PlaybackFacts | undefined,
  wasmAudio = false,
): PlaybackDifficulty | null {
  if (!info?.playMethod) return null;
  const raw = info.transcodeReasons ?? [];

  if (info.playMethod === "PLAY_METHOD_DIRECT_PLAY") return withDifficulty("direct", raw);
  // Container-only mismatch: bytes are copied, only the wrapper is rebuilt.
  if (info.playMethod === "PLAY_METHOD_DIRECT_STREAM") return withDifficulty("remux", raw);

  // Transcoding. Video is what actually costs - an audio-only re-encode is
  // cheap enough that lumping it with a 4K tonemap would make the badge useless.
  if (info.videoDecision === "transcode") {
    const height = info.videoHeight ?? 0;
    // Tone mapping is expensive whatever the size, and without a GPU tonemap it
    // forces the software-hdr ladder class - the costliest route there is.
    if (raw.includes("hdr_tonemapping_needed") || height >= HEAVY_HEIGHT) {
      return withDifficulty("heavy", raw);
    }
    // height 0 means the stream row carries no dimensions; assume HD rather than
    // claiming an encode is cheap on no evidence.
    if (height > 0 && height <= SD_MAX_HEIGHT) return withDifficulty("videoSd", raw);
    return withDifficulty("video", raw);
  }

  // Video is copied. The server says "transcode" for an AC-3/E-AC-3 track only
  // because getBrowserCapabilities deliberately hides ac3/eac3 (advertising them
  // would make the server mux a stream MSE then rejects). With ?wasmAudio=1 the
  // track is served as a COPIED out-of-band rendition that ac3go decodes in the
  // browser, so nothing is re-encoded server-side - counting it as an audio
  // transcode overstated the cost.
  const audioViaWasm = wasmAudio && isWasmDecodableAudioCodec(info.audioCodec);
  // Only the codec reason is annulled: a channel-target or resync request is
  // real work the wasm path does not absorb.
  const reasons = audioViaWasm ? raw.filter((r) => r !== "audio_codec_unsupported") : raw;

  // A container mismatch is NOT an encode - it is the repackage the server
  // itself treats as direct-stream-eligible (isDirectStreamEligible only tests
  // onlyContainerIssue). Counting it as leftover work made the real-world Chrome
  // case - MKV + E-AC-3, which returns BOTH audio_codec_unsupported and
  // container_incompatible - report an audio transcode that never happens.
  const encodesLeft = reasons.filter((r) => r !== "container_incompatible");

  if (encodesLeft.length === 0) return withDifficulty(audioViaWasm ? "wasmAudio" : "remux", raw);
  return withDifficulty("audioOnly", encodesLeft);
}

/** Worth flagging on a poster tile. A grid badges only the titles that will
    make the server work hard - marking the easy ones too would turn a warning
    into wallpaper. */
export function isHeavyToStream(d?: PlaybackDifficulty | null): boolean {
  return d?.difficulty === "demanding" || d?.difficulty === "hard";
}
