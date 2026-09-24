// Pure room-state reconciliation for SyncPlay on TV. No React, no I/O - kept
// isolated so the drift-tolerance rule (the actual bug risk: over-correcting
// stutters the picture, under-correcting drifts everyone apart) is unit
// tested on its own, independent of the websocket hook or the player screen.

// A SyncPlay "play"/"pause"/"state" message off the wire carries the room's
// target position at the moment it was sent. Snapping the local playhead to
// that exact millisecond on every message would fight small, harmless
// scheduling jitter - network RTT, the player's own timeUpdate tick
// (500ms on this screen, see [id].tsx), rounding in the server's
// elapsed-time projection for a mid-session joiner (hub.go ConnectMember) -
// and stutter the picture on every single sync message.
//
// SYNCPLAY_DRIFT_THRESHOLD_MS is the largest drift the TV tolerates before
// correcting the position: below it, only the play/pause transition is
// applied; at or above it, the local position snaps to the room's target.
// 1000ms is a deliberate middle ground - comfortably above the jitter
// sources above (tens to a few hundred milliseconds), while still tight
// enough that "watching together" stays meaningfully in sync: nobody on a
// couch notices being under a second ahead or behind, but several seconds
// off defeats the point of watching together.
export const SYNCPLAY_DRIFT_THRESHOLD_MS = 1000;

export interface ReconcilePositionInput {
  localPositionMs: number;
  targetPositionMs: number;
  thresholdMs?: number;
}

export interface ReconcilePositionResult {
  // Whether the caller should move the player's position at all. false means
  // "leave the playhead alone", not "ignore the message" - the play/pause
  // transition itself (if any) must still be applied.
  shouldSeek: boolean;
  // The position to land on: the target when correcting, the unchanged local
  // position otherwise (so a caller can always just do
  // `player.currentTime = positionMs / 1000` without a branch).
  positionMs: number;
}

export function reconcilePosition({
  localPositionMs,
  targetPositionMs,
  thresholdMs = SYNCPLAY_DRIFT_THRESHOLD_MS,
}: ReconcilePositionInput): ReconcilePositionResult {
  const driftMs = Math.abs(targetPositionMs - localPositionMs);
  if (driftMs <= thresholdMs) {
    return { shouldSeek: false, positionMs: localPositionMs };
  }
  return { shouldSeek: true, positionMs: targetPositionMs };
}

// Re-exported, not restated: the shape of a room code is a security parameter
// the server owns, and @evey/api pins it to the Go source (roomCode.guard).
// Validated client-side so a malformed code fails the remote's keypad instantly
// instead of round tripping for something that can never be a real room.
export { ROOM_CODE_LENGTH, ROOM_CODE_ALPHABET, isValidRoomCode, normalizeRoomCode } from "@evey/api";
