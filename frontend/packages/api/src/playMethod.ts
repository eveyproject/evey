// The server names the play method BEFORE it knows what the client will really
// do with the stream. A browser that decodes AC-3/E-AC-3 in WebAssembly never
// advertises those codecs (advertising them would make the server COPY them
// into the muxed stream, which MSE then rejects), so the decision engine flags
// `audio_codec_unsupported`, sets audio_decision=transcode and lands on
// play_method=transcode - while the actual session copies the video, copies the
// audio into an out-of-band rendition, and only swaps the container.
//
// Read alone, that verdict made the stream-info panel contradict itself:
//
//   Méthode      Transcode
//   Vidéo        copié
//   Audio        décodé client
//   Raisons du transcodage - Conteneur non lisible en l'état (remis en HLS)
//
// The per-track decisions are the ground truth: it is a transcode only when a
// track is actually re-encoded. Every surface that names the method (web,
// mobile and TV stream info; the admin sessions list and now-playing panel)
// derives it here rather than growing its own patch - the now-playing panel
// already carried one for the wasm case that the sessions list lacked.

export type PlayMethodKind = "direct_play" | "direct_stream" | "transcode";

// Both wire spellings reach the UI: the proto enum the playback API returns
// (PLAY_METHOD_TRANSCODE) and the snake_case the session API stores
// ("transcode"). Anything else - including the empty method a cast-resurrected
// or music session carries - normalizes to "".
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

// "wasm" (the ac3go client-side decode, which the cast receiver's audio_mode
// beacon also writes back onto the session) means the track is served
// untouched - a copy, not a re-encode.
function isReencoded(decision?: string): boolean {
  return (decision ?? "").trim().toLowerCase() === "transcode";
}

export interface PlayMethodInput {
  playMethod?: string;
  videoDecision?: string;
  audioDecision?: string;
  // wasmAudio: this client is decoding the audio itself, so the server's
  // "transcode" verdict on that track never happens. Only the web player knows
  // it (it is a runtime state, not a field), hence the explicit flag.
  wasmAudio?: boolean;
}

// effectivePlayMethod returns the method the session is REALLY running, which
// is the declared one corrected by the per-track decisions. Returns "" when
// there is no method to name at all.
export function effectivePlayMethod(input: PlayMethodInput): PlayMethodKind | "" {
  const declared = normalizePlayMethod(input.playMethod);
  // Direct play touches nothing, so there is nothing to correct; and a session
  // without a declared method must stay unlabelled rather than gain a badge.
  if (declared === "" || declared === "direct_play") return declared;

  const video = (input.videoDecision ?? "").trim();
  const audio = (input.audioDecision ?? "").trim();
  // Older sessions predate the per-track fields: with no ground truth, the
  // server's verdict is the best answer available.
  if (!video && !audio) return declared;

  if (isReencoded(video) || (isReencoded(audio) && !input.wasmAudio)) return "transcode";
  // Nothing re-encoded: whatever kept this off direct play is a container swap.
  return "direct_stream";
}

// isRealTranscode reports that at least one track is being re-encoded - the
// only case that deserves the costly-looking warning colour, and the only case
// where "transcode reasons" is an honest heading for the reason list.
export function isRealTranscode(input: PlayMethodInput): boolean {
  return effectivePlayMethod(input) === "transcode";
}

// These two only routed the audio OFF direct play and onto the wasm rendition.
// Nothing is transcoded because of them, so they must not be listed as reasons
// for a transcode that isn't happening.
const WASM_MOOT_REASONS = ["audio_codec_unsupported", "audio_channels_exceeded"];

export function visibleTranscodeReasons(reasons?: string[], wasmAudio?: boolean): string[] {
  const all = reasons ?? [];
  if (!wasmAudio) return all;
  return all.filter((r) => !WASM_MOOT_REASONS.includes(r));
}

// playMethodI18nKey maps a kind onto the shared player labels. "" yields ""
// so callers can drop the row (or badge) instead of rendering a blank one.
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

// reasonsHeadingI18nKey names the reason list for what it actually is: reasons
// for a transcode when something is re-encoded, otherwise reasons why direct
// play was impossible (a remux is not a transcode).
export function reasonsHeadingI18nKey(input: PlayMethodInput): string {
  return isRealTranscode(input)
    ? "player.streamInfo.transcodeReasons"
    : "player.streamInfo.noDirectPlayReasons";
}
