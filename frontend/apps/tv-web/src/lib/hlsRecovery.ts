/**
 * What the player does with a FATAL hls.js error. Recovery used to be
 * unbounded: a media error was "recovered" and a network error reloaded, again
 * and again, so a stream the set cannot decode (manifestIncompatibleCodecsError)
 * left the viewer on a frozen 0:00 with no message and no way out but Back.
 */
export type HlsFatalAction = "renegotiate" | "reload" | "recoverMedia" | "fail";

export interface HlsFatalError {
  type: string; // hls.js ErrorTypes value: "networkError" | "mediaError" | ...
  details: string;
  responseCode?: number;
}

/** Attempts already spent on this stream, per kind of recovery. */
export interface HlsRecoveryAttempts {
  reload: number;
  recoverMedia: number;
}

const MAX_RELOADS = 3;
const MAX_MEDIA_RECOVERIES = 1;

// No retry changes a decoder that cannot play the codec it was handed.
const UNRECOVERABLE = new Set(["manifestIncompatibleCodecsError"]);

export function decideHlsFatal(err: HlsFatalError, attempts: HlsRecoveryAttempts): HlsFatalAction {
  // A 410 is the server saying the master expired (a sealed fingerprint):
  // negotiate again rather than hammer the dead url.
  if (err.responseCode === 410) return "renegotiate";
  if (UNRECOVERABLE.has(err.details)) return "fail";
  if (err.type === "networkError") return attempts.reload < MAX_RELOADS ? "reload" : "fail";
  if (err.type === "mediaError") return attempts.recoverMedia < MAX_MEDIA_RECOVERIES ? "recoverMedia" : "fail";
  return "fail";
}
