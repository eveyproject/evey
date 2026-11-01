import type { PlayerEventDiag } from "@evey/api";

// playerErrorDiag - a native player error, taken apart for the server log.
//
// expo-video hands JS a single string built from ExoPlayer's
// PlaybackException: "A playback exception has occurred: " + its message +
// " " + its first cause's message. For a renderer failure that message names
// the renderer, the whole format it was fed and ExoPlayer's own verdict on it
// (format_supported=YES, NO_EXCEEDS_CAPABILITIES...), and the cause names the
// decoder. The server used to keep the first 200 characters, which stopped
// before the verdict and the cause. Diagnostic only: nothing here changes how
// a stream is played.

const PREFIX = "A playback exception has occurred:";

function rendererOf(msg: string): string {
  const m = /\b(\w*)Renderer error\b/.exec(msg);
  if (!m) return "";
  const name = m[1].toLowerCase();
  if (name.includes("video")) return "video";
  if (name.includes("audio")) return "audio";
  if (name.includes("text") || name.includes("subtitle")) return "text";
  return name;
}

/** Fields of the first "Format(...)": id, label, container, sample mime, codecs, bitrate, language, [w, h, fps, ...]. */
function formatOf(msg: string): Pick<PlayerEventDiag, "mime" | "codecs" | "width" | "height" | "fps" | "bitrateKbps"> {
  const out = { mime: "", codecs: "", width: 0, height: 0, fps: 0, bitrateKbps: 0 };
  const start = msg.indexOf("Format(");
  if (start < 0) return out;
  const body = msg.slice(start + "Format(".length);
  const fields = body.split(", ");
  const val = (i: number) => (fields[i] && fields[i] !== "null" ? fields[i] : "");
  out.mime = val(3);
  out.codecs = val(4);
  const bitrate = Number(val(5));
  if (Number.isFinite(bitrate) && bitrate > 0) out.bitrateKbps = Math.floor(bitrate / 1000);
  const video = /\[(-?\d+), (-?\d+), (-?[\d.]+)/.exec(body);
  if (video) {
    const [w, h, fps] = [Number(video[1]), Number(video[2]), Number(video[3])];
    if (w > 0) out.width = w;
    if (h > 0) out.height = h;
    if (fps > 0) out.fps = fps;
  }
  return out;
}

export function diagnoseNativePlayerError(message: string, playMethod: string): PlayerEventDiag {
  const msg = (message ?? "").startsWith(PREFIX) ? message.slice(PREFIX.length).trim() : (message ?? "").trim();
  const verdict = /format_supported=(\w+)/.exec(msg);
  // Past the verdict comes the cause's message; without a verdict (a source
  // or a timeout error) the whole message is the cause.
  const rest = verdict ? msg.slice(verdict.index + verdict[0].length).trim() : msg;
  const decoder = /\b((?:c2|OMX)\.[\w.-]+?)(?=[,\s]|$)/.exec(msg);
  const code = /\bERROR_CODE_\w+/.exec(msg);
  return {
    renderer: rendererOf(msg),
    formatSupported: verdict ? verdict[1] : "",
    decoder: decoder ? decoder[1] : "",
    ...formatOf(msg),
    playMethod,
    errorCode: code ? code[0] : "",
    causes: rest ? [rest] : [],
  };
}

/** The stream info's PLAY_METHOD_* as the log names it; "offline" for a downloaded file. */
export function playMethodName(playMethod: string | undefined, offline = false): string {
  if (offline) return "offline";
  switch (playMethod) {
    case "PLAY_METHOD_DIRECT_PLAY":
      return "direct_play";
    case "PLAY_METHOD_DIRECT_STREAM":
      return "direct_stream";
    case "PLAY_METHOD_TRANSCODE":
      return "transcode";
    default:
      return "";
  }
}
