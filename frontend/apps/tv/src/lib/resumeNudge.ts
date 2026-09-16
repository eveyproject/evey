// A native player resumed after a long pause starts its audio clock before the
// video renderer has a frame (the codec restarts from the last keyframe): a
// frozen picture with the sound running, then a jump. A seek in place on the
// resume makes the player wait for that first frame before anything runs.
export const RESUME_NUDGE_MIN_PAUSE_MS = 4000;
const NUDGE_OFFSET_SEC = 0.01;

export function shouldNudgeOnResume(pausedForMs: number): boolean {
  return pausedForMs >= RESUME_NUDGE_MIN_PAUSE_MS;
}

// resumeNudger tracks pause/play transitions and seeks in place on a resume
// that qualifies. Feed it every playingChange; it ignores the rest.
export function resumeNudger(seekBy: (sec: number) => void, now: () => number = Date.now) {
  let pausedAt: number | null = null;
  return (playing: boolean): boolean => {
    if (!playing) {
      if (pausedAt === null) pausedAt = now();
      return false;
    }
    const since = pausedAt === null ? 0 : now() - pausedAt;
    pausedAt = null;
    if (!shouldNudgeOnResume(since)) return false;
    seekBy(NUDGE_OFFSET_SEC);
    return true;
  };
}
