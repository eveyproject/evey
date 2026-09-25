import { describe, expect, it } from "vitest";
import { firstStartMs } from "./resumePosition";

describe("firstStartMs", () => {
  const watched = { positionMs: "65000", durationMs: "600000", played: true };

  it("starts a title already watched over, on the first stream only", () => {
    const first = { current: true };
    expect(firstStartMs(first, undefined, watched)).toBe(0);
    // The quality switch that follows continues at the saved position.
    expect(firstStartMs(first, undefined, watched)).toBe(65000);
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
