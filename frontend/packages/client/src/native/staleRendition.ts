// The server answers 410 to a URL it no longer serves as the player holds it:
// a rendition URL sealed with a decision fingerprint that has since moved (a
// ladder reshaped, a new server build), or an od_ URL of a file since replaced.
// The web player rebuilds at its playhead on the first one; the native players
// used to stop on the error screen (a phone at 11632 s, 2026-10-13).
//
// ExoPlayer gives no typed status, only its message, measured on that phone:
// "A playback exception has occurred: Source error Response code: 410".
const STALE_RESPONSE_RE = /response code:\s*410\b/i;

// A second 410 this soon after a rebuild is not a stale URL any more - the
// fresh master failed the same way - and is shown, like the web's second one.
export const STALE_RETRY_WINDOW_MS = 60_000;

// staleRenditionRetry says whether a native playback error is a stale URL worth
// one rebuild at the playhead. lastRetryAtMs is when the previous one was made.
export function staleRenditionRetry(message: string | undefined, lastRetryAtMs: number | null, nowMs: number): boolean {
  if (!message || !STALE_RESPONSE_RE.test(message)) return false;
  return lastRetryAtMs === null || nowMs - lastRetryAtMs >= STALE_RETRY_WINDOW_MS;
}

// The part of expo-video's VideoPlayer the rebuild uses.
export interface ReloadablePlayer<S> {
  replaceAsync(source: S): Promise<void>;
  currentTime: number;
  play(): void;
}

// reloadAtPlayhead loads the same source again and goes back to sec. The
// master URL carries no seal: fetched again, it comes back with fresh sealed
// URLs, so the play session, its token and the selected tracks all stay.
export async function reloadAtPlayhead<S>(player: ReloadablePlayer<S>, source: S, sec: number): Promise<void> {
  await player.replaceAsync(source);
  player.currentTime = sec;
  player.play();
}
