// Subtitle resync primitives - pure math only. NOT wired to the player yet:
// expo-video exposes subtitle TRACK SELECTION only (SubtitleTrack has no cue
// accessor, see expo-video's VideoPlayer.types), so there is no live cue list
// to shift the way the web's useSubtitleManager mutates TextTrackCue objects.
// These functions are the primitive a future client-side WebVTT renderer
// would need (see the mobile player dispatch report for the renderer plan);
// kept here, tested, so that renderer doesn't have to re-derive the math.

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
