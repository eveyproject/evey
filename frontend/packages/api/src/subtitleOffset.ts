// Subtitle resync primitives - pure math only, no DOM and no React, which is
// why they sit in the shared package: the mobile and TV renderers both shift
// cues, and the web does the same arithmetic on live TextTrackCue objects in
// useSubtitleManager. Kept separate from subtitleCues.ts because this half only
// knows {start,end} pairs - it has no opinion on what a cue carries.

export const SUBTITLE_OFFSET_STEP_MS = 100;
export const SUBTITLE_OFFSET_MAX_MS = 10_000;

export function clampSubtitleOffsetMs(ms: number): number {
  return Math.max(-SUBTITLE_OFFSET_MAX_MS, Math.min(SUBTITLE_OFFSET_MAX_MS, ms));
}

export interface Cue {
  start: number;
  end: number;
}

// applyCueOffset shifts every cue's SOURCE time by offsetSec, clamping the
// start at 0 (a cue can't display before the video begins) while keeping
// end >= start. Mirrors the web's applySubtitleOffset (useSubtitleManager.ts)
// but operates on plain {start,end} pairs instead of live TextTrackCue
// objects, so it works against parsed VTT cues with no DOM dependency.
export function applyCueOffset(cues: Cue[], offsetSec: number): Cue[] {
  return cues.map((cue) => {
    const start = Math.max(0, cue.start + offsetSec);
    const end = Math.max(start, cue.end + offsetSec);
    return { start, end };
  });
}
