// Pure position math for a seek driven by a remote, shared by every
// television player (the Android TV app, the browser client). Kept free of
// React and of any platform so it is trivial to unit test and so a player
// only has to call it, not re-derive it.
//
// A remote maps left/right to a single skip (the same skipBackward/
// skipForwardSeconds used everywhere else in the app) and a HELD key to a
// scrub whose rate climbs with the hold. Repeated taps grow too.
export const SEEK_ACCELERATION_MULTIPLIER = 4;

export type SeekKeyType = "left" | "right" | "longLeft" | "longRight";

// Signed seconds to move the playhead by for a given remote key: negative is
// backward. Returns 0 for anything that isn't a seek key so callers can pass
// arbitrary event types through without a separate guard.
export function seekDeltaForKey(type: string, skipBackwardSec: number, skipForwardSec: number): number {
  switch (type as SeekKeyType) {
    case "left":
      return -skipBackwardSec;
    case "right":
      return skipForwardSec;
    case "longLeft":
      return -skipBackwardSec * SEEK_ACCELERATION_MULTIPLIER;
    case "longRight":
      return skipForwardSec * SEEK_ACCELERATION_MULTIPLIER;
    default:
      return 0;
  }
}

// Position math, not clock math: the result is clamped to the known timeline so a
// hold near either edge cannot preview (or seek) outside [0, duration]. A duration
// of 0 (not yet known) leaves the position unclamped upward - there is nothing
// sane to clamp to yet.
export function clampSeekTarget(currentSec: number, deltaSec: number, durationSec: number): number {
  const target = currentSec + deltaSec;
  if (target < 0) return 0;
  if (durationSec > 0 && target > durationSec) return durationSec;
  return target;
}

// The progress bar shows a locally-computed preview position the instant a seek key
// is pressed (instead of waiting for the player's next timeUpdate tick) so holding
// left/right reads as "the bar is moving", not "decorative". Once the real playhead
// (fed by the player's timeUpdate listener) catches up within tolerance, the preview
// is released and the bar goes back to tracking the player directly - otherwise a
// slightly-off native keyframe-snap would freeze the bar just short of the target.
export function shouldReleaseSeekPreview(actualSec: number, previewSec: number, toleranceSec = 2): boolean {
  return Math.abs(actualSec - previewSec) <= toleranceSec;
}

// Repeated taps in one gesture grow: the first two are the plain skip, the
// next two double it, the next two double it again, up to eight times the
// skip. A viewer who wants ten minutes ahead used to need twenty taps of
// thirty seconds and read the bar as "stuck at thirty seconds" (owner,
// 2026-09-22); now eight taps cover it, and the first ones stay fine-grained
// so a tap or two still lands where a plain skip would.
export const TAP_SEEK_MAX_MULTIPLIER = 8;

export function tapSeekStep(pressCount: number, skipSec: number): number {
  const n = Math.max(1, Math.floor(pressCount));
  const multiplier = Math.min(TAP_SEEK_MAX_MULTIPLIER, 2 ** Math.floor((n - 1) / 2));
  return skipSec * multiplier;
}

// The rate a HELD key scrubs at, in seconds of media per second of holding.
// It climbs with the hold - a minute per second at first so a short hold
// stays precise, then four minutes, then ten - so a two-hour film crosses in
// about fifteen seconds while a ten-second hold still lands within a scene.
export function holdScrubRate(heldMs: number): number {
  if (heldMs < 1000) return 60;
  if (heldMs < 3000) return 240;
  return 600;
}

// How long the presses may keep coming before the pending seek commits: one
// seek for a burst of taps, not one restart of the stream per tap.
export const SEEK_COMMIT_DELAY_MS = 600;
// How often a hold moves the preview.
export const HOLD_SCRUB_TICK_MS = 100;
