// Firefox parks a paused video's decoder after a few seconds. On resume the
// element's clock (system time on a video-only MSE element) runs at once while
// no frame is decoded: a frozen picture, sound going on, until hls.js's 3 s
// watchdog nudge-seeks and the decoder restarts - the picture then skips what
// the clock already spent. Seeking in place on the resume gives the decoder its
// restart before the clock moves, so picture and sound leave together.
export const RESUME_NUDGE_MIN_PAUSE_MS = 4000;
const NUDGE_OFFSET_SEC = 0.01;

export function shouldNudgeOnResume(pausedForMs: number, seeking: boolean, readyState: number): boolean {
  return pausedForMs >= RESUME_NUDGE_MIN_PAUSE_MS && !seeking && readyState >= 2;
}

export function nudgeOnResume(video: HTMLVideoElement, pausedAt: number | null, now = Date.now()): boolean {
  if (pausedAt === null || !shouldNudgeOnResume(now - pausedAt, video.seeking, video.readyState)) return false;
  video.currentTime = video.currentTime + NUDGE_OFFSET_SEC;
  return true;
}
