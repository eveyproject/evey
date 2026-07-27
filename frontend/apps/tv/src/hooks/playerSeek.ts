// Pure position math for the TV remote seek gesture. Kept free of React/RN so it
// is trivial to unit test and so the screen only has to call it, not re-derive it.
//
// The remote maps left/right to a single skip (the same skipBackward/
// skipForwardSeconds used everywhere else in the app) and longLeft/longRight -
// native react-native-tvos "hold" events, not a hand-rolled repeat-timer - to a
// bigger accelerated jump. Using the platform's own hold event means we don't have
// to guess a repeat-rate threshold per remote/emulator.
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
