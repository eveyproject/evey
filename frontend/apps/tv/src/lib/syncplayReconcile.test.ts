import { describe, it, expect } from "vitest";
import { reconcilePosition, isValidRoomCode, SYNCPLAY_DRIFT_THRESHOLD_MS, ROOM_CODE_LENGTH } from "./syncplayReconcile";

describe("reconcilePosition", () => {
  it("does not correct a drift under the threshold", () => {
    const result = reconcilePosition({ localPositionMs: 10_000, targetPositionMs: 10_300 });
    expect(result).toEqual({ shouldSeek: false, positionMs: 10_000 });
  });

  it("does not correct a drift exactly at the threshold (boundary is inclusive)", () => {
    const result = reconcilePosition({
      localPositionMs: 10_000,
      targetPositionMs: 10_000 + SYNCPLAY_DRIFT_THRESHOLD_MS,
    });
    expect(result.shouldSeek).toBe(false);
  });

  it("corrects a drift above the threshold, landing exactly on the target", () => {
    const result = reconcilePosition({ localPositionMs: 10_000, targetPositionMs: 15_000 });
    expect(result).toEqual({ shouldSeek: true, positionMs: 15_000 });
  });

  it("corrects symmetrically when the local playhead is AHEAD of the room", () => {
    const result = reconcilePosition({ localPositionMs: 20_000, targetPositionMs: 10_000 });
    expect(result).toEqual({ shouldSeek: true, positionMs: 10_000 });
  });

  it("honours a custom threshold override", () => {
    const result = reconcilePosition({ localPositionMs: 1000, targetPositionMs: 1400, thresholdMs: 500 });
    expect(result.shouldSeek).toBe(false);
  });

  it("treats a large initial-join drift (fresh player at 0) as a correction", () => {
    // The first "state" message after connecting mid-session carries the
    // room's live position while a freshly opened player is still near 0 -
    // this must always resync, which falls naturally out of the same rule.
    const result = reconcilePosition({ localPositionMs: 0, targetPositionMs: 632_000 });
    expect(result).toEqual({ shouldSeek: true, positionMs: 632_000 });
  });
});

describe("isValidRoomCode", () => {
  it(`accepts a ${ROOM_CODE_LENGTH}-character code`, () => {
    expect(isValidRoomCode("ABC123")).toBe(true);
  });

  it("rejects a code that is too short", () => {
    expect(isValidRoomCode("ABC12")).toBe(false);
  });

  it("rejects a code that is too long", () => {
    expect(isValidRoomCode("ABC1234")).toBe(false);
  });

  it("rejects an empty code", () => {
    expect(isValidRoomCode("")).toBe(false);
  });

  // The remote's keypad only emits alphabet characters, but the same helper
  // guards a pasted code on the other clients.
  it("folds what a viewer reads off a screen", () => {
    expect(isValidRoomCode("abc-123")).toBe(true);
    expect(isValidRoomCode("ABCI23")).toBe(true);
  });
});
