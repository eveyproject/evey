import { describe, expect, it } from "vitest";
import { firstStartMs, resolveStartPositionMs, resumableMs } from "./resumePosition";

describe("firstStartMs", () => {
  const watched = { positionMs: "65000", durationMs: "600000", played: true };
  const finished = { positionMs: "560000", durationMs: "600000", played: true };

  it("starts a title finished at its credits over, on the first stream only", () => {
    const first = { current: true };
    expect(firstStartMs(first, undefined, finished)).toBe(0);
    // The quality switch that follows continues at the saved position.
    expect(firstStartMs(first, undefined, finished)).toBe(560000);
  });

  it("resumes a title watched again where the new viewing stopped", () => {
    expect(firstStartMs({ current: true }, undefined, watched)).toBe(65000);
  });

  it("keeps the end a trap on a switch too", () => {
    const later = { current: false };
    expect(firstStartMs(later, undefined, { positionMs: "595000", durationMs: "600000", played: true })).toBe(0);
  });

  it("honours restart and the cast position on the first stream only", () => {
    const first = { current: true };
    expect(firstStartMs(first, { restart: true }, { positionMs: "65000", durationMs: "600000" })).toBe(0);
    expect(firstStartMs(first, { restart: true }, { positionMs: "65000", durationMs: "600000" })).toBe(65000);
    expect(firstStartMs({ current: true }, { startSec: 42 }, watched)).toBe(42000);
  });
});

describe("resolveStartPositionMs on a title watched again", () => {
  // Completion is sticky on the server: a film finished once keeps its played
  // flag while the position follows the new viewing. Leaving that viewing and
  // pressing Resume must land where the viewer left, not at the start.
  it("resumes a played title stopped before its completion point", () => {
    expect(resolveStartPositionMs(false, { positionMs: "1800000", durationMs: "7200000", played: true })).toBe(1800000);
    expect(resumableMs({ positionMs: "1800000", durationMs: "7200000", played: true })).toBe(1800000);
  });

  it("still starts over a played title stopped in its credits or with no known length", () => {
    expect(resolveStartPositionMs(false, { positionMs: "6600000", durationMs: "7200000", played: true })).toBe(0);
    expect(resolveStartPositionMs(false, { positionMs: "1800000", played: true })).toBe(0);
    expect(resumableMs({ positionMs: "0", durationMs: "7200000", played: true })).toBe(0);
  });
});
