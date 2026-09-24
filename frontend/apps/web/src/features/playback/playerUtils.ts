// nominalQualityTier lives in @evey/api now: mobile needs the same rounding to
// read the same manifest, and a second copy of a rule like this is how the two
// players end up labelling the same rung differently. Re-exported here so the
// call sites in this folder keep importing from where they always did.
export { nominalQualityTier } from "@evey/api";

// Where closing the player lands: shared with the phone and the television
// (packages/client/src/lib/playerExit.ts), re-exported for this folder.
export { playerCloseTarget, playerExit, type PlayerExit } from "@evey/client";

// Where playback begins: shared with the phone and the televisions
// (packages/client/src/lib/resumePosition.ts).
export { resolveStartPositionMs } from "@evey/client";

