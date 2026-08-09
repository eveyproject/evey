// play_method ROUTES (it picks the URL shape); the per-track decisions say what
// was done to the bytes. Naming the method from the enum alone made the panel
// read "Transcode" over a copied video and a client-decoded audio. Every
// surface that names it derives it here.

export type PlayMethodKind = "direct_play" | "direct_stream" | "transcode";

// Both wire spellings reach the UI: the proto enum and the snake_case the
// session API stores. Anything else - including the empty method a
// cast-resurrected or music session carries - normalizes to "".
export function normalizePlayMethod(method?: string): PlayMethodKind | "" {
  switch ((method ?? "").trim().toLowerCase()) {
    case "play_method_direct_play":
    case "direct_play":
      return "direct_play";
    case "play_method_direct_stream":
    case "direct_stream":
      return "direct_stream";
    case "play_method_transcode":
    case "transcode":
      return "transcode";
    default:
      return "";
  }
}

// "wasm" means served untouched for a client-side decode - a copy.
function isReencoded(decision?: string): boolean {
  return (decision ?? "").trim().toLowerCase() === "transcode";
}

export interface PlayMethodInput {
  playMethod?: string;
  videoDecision?: string;
  audioDecision?: string;
  // Live override for the fallback case: the server already answers "wasm" in
  // audioDecision, but the web decoder can latch off mid-session.
  wasmAudio?: boolean;
}

export function effectivePlayMethod(input: PlayMethodInput): PlayMethodKind | "" {
  const declared = normalizePlayMethod(input.playMethod);
  // Direct play touches nothing; an undeclared method must stay unlabelled.
  if (declared === "" || declared === "direct_play") return declared;

  const video = (input.videoDecision ?? "").trim();
  const audio = (input.audioDecision ?? "").trim();
  // Sessions predating the per-track fields: the verdict is all there is.
  if (!video && !audio) return declared;

  if (isReencoded(video) || (isReencoded(audio) && !input.wasmAudio)) return "transcode";
  return "direct_stream";
}

export function isRealTranscode(input: PlayMethodInput): boolean {
  return effectivePlayMethod(input) === "transcode";
}

// Moot on the wasm path: they routed the audio onto the rendition, they did not
// cause a transcode. The server already drops them; this covers the fallback.
const WASM_MOOT_REASONS = ["audio_codec_unsupported", "audio_channels_exceeded"];

export function visibleTranscodeReasons(reasons?: string[], wasmAudio?: boolean): string[] {
  const all = reasons ?? [];
  if (!wasmAudio) return all;
  return all.filter((r) => !WASM_MOOT_REASONS.includes(r));
}

// "" yields "" so callers drop the row instead of rendering a blank one.
export function playMethodI18nKey(kind: PlayMethodKind | ""): string {
  switch (kind) {
    case "direct_play":
      return "player.streamInfo.methodDirectPlay";
    case "direct_stream":
      return "player.streamInfo.methodDirectStream";
    case "transcode":
      return "player.streamInfo.methodTranscode";
    default:
      return "";
  }
}

// A remux is not a transcode: its reasons are why direct play was impossible.
export function reasonsHeadingI18nKey(input: PlayMethodInput): string {
  return isRealTranscode(input)
    ? "player.streamInfo.transcodeReasons"
    : "player.streamInfo.noDirectPlayReasons";
}
