import { describe, expect, it } from "vitest";
import { PLAYER_BUFFER_MIN_BYTES, PLAYER_BUFFER_UNCAPPED_HEAP_MB, playerBufferOptions } from "./playerBufferOptions";

const MIB = 1024 * 1024;

describe("playerBufferOptions", () => {
  it("caps a small television's player at an eighth of its heap", () => {
    expect(playerBufferOptions({ memoryClassMb: 128 })).toEqual({ maxBufferBytes: 16 * MIB, prioritizeTimeOverSizeThreshold: false });
    expect(playerBufferOptions({ memoryClassMb: 256 })?.maxBufferBytes).toBe(32 * MIB);
  });
  it("never goes under the floor, and stays under 64 MiB up to the uncapped heap class", () => {
    expect(playerBufferOptions({ memoryClassMb: 48 })?.maxBufferBytes).toBe(PLAYER_BUFFER_MIN_BYTES);
    expect(playerBufferOptions({ memoryClassMb: 500 })?.maxBufferBytes).toBe(500 * MIB / 8);
    const largestCapped = playerBufferOptions({ memoryClassMb: PLAYER_BUFFER_UNCAPPED_HEAP_MB - 1 })?.maxBufferBytes;
    expect(largestCapped).toBeLessThan(64 * MIB);
  });
  it("leaves a large heap, or an unknown device, to the player's own sizing", () => {
    expect(playerBufferOptions({ memoryClassMb: 512 })).toBeUndefined();
    expect(playerBufferOptions(null)).toBeUndefined();
    expect(playerBufferOptions({ memoryClassMb: 0 })).toBeUndefined();
  });
});
