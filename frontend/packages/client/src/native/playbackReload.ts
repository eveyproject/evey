// Two answers of the server to a playback resource mean "load the master
// again", not "this film is broken":
//   - 410: a URL it no longer serves as the player holds it - a rendition URL
//     sealed with a decision fingerprint that has since moved (a ladder
//     reshaped, a new server build), or an od_ URL of a file since replaced;
//   - 404: a resource that is gone - segments evicted from the cache while the
//     viewer was paused, a session lost to a server restart.
// The web player rebuilds at its playhead on the first one; the native players
// used to stop on the error screen (a phone at 11632 s, 2026-10-13).
//
// expo-video gives no typed status (PlayerError is { message } only, 55.0.16):
// on Android the message is "A playback exception has occurred: " + Media3's
// message + its cause's, and the status lives in the cause's text, measured on
// that phone: "... Source error Response code: 410". DEV.md's dependency
// checklist has this text re-checked on every expo-video or Media3 upgrade.
const RELOAD_RESPONSE_RE = /response code:\s*(404|410)\b/i;

// A second one this soon after a reload is not a passing state any more - the
// fresh master failed the same way - and is shown, like the web's second one.
export const RELOAD_RETRY_WINDOW_MS = 60_000;

// reloadableStatus is the HTTP status a native playback error carries when it
// is one a reload cures, null otherwise.
export function reloadableStatus(message: string | undefined): number | null {
  const m = message ? RELOAD_RESPONSE_RE.exec(message) : null;
  return m ? Number(m[1]) : null;
}

// playbackReloadRetry says whether a native playback error is worth one reload
// at the playhead. lastRetryAtMs is when the previous one was made, 404 or 410.
export function playbackReloadRetry(message: string | undefined, lastRetryAtMs: number | null, nowMs: number): boolean {
  if (reloadableStatus(message) === null) return false;
  return lastRetryAtMs === null || nowMs - lastRetryAtMs >= RELOAD_RETRY_WINDOW_MS;
}

// The part of expo-video's VideoPlayer the reload uses.
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
