// Shared implementation lives in @evey/client (packages/client/src/lib/seekGesture.ts):
// the browser TV client drives its seek with the same rules. This module
// keeps the path the player screen and its tests import.
export {
  SEEK_ACCELERATION_MULTIPLIER,
  TAP_SEEK_MAX_MULTIPLIER,
  seekDeltaForKey,
  clampSeekTarget,
  shouldReleaseSeekPreview,
  tapSeekStep,
  holdScrubRate,
} from "@evey/client";
export type { SeekKeyType } from "@evey/client";
