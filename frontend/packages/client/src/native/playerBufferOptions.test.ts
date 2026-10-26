import { describe, expect, it } from "vitest";
import { PLAYER_BUFFER_MAX_BYTES, PLAYER_BUFFER_MIN_BYTES, playerBufferOptions } from "./playerBufferOptions";

const MIB = 1024 * 1024;

describe("playerBufferOptions", () => {
  it("caps a small television's player at a quarter of its heap", () => {
    expect(playerBufferOptions({ memoryClassMb: 128 })).toEqual({ maxBufferBytes: 32 * MIB, prioritizeTimeOverSizeThreshold: false });
    expect(playerBufferOptions({ memoryClassMb: 256 })?.maxBufferBytes).toBe(64 * MIB);
  });
  it("never goes under the floor nor over the ceiling", () => {
    expect(playerBufferOptions({ memoryClassMb: 48 })?.maxBufferBytes).toBe(PLAYER_BUFFER_MIN_BYTES);
    expect(playerBufferOptions({ memoryClassMb: 384 })?.maxBufferBytes).toBe(PLAYER_BUFFER_MAX_BYTES);
  });
  it("leaves a large heap, or an unknown device, to the player's own sizing", () => {
    expect(playerBufferOptions({ memoryClassMb: 512 })).toBeUndefined();
    expect(playerBufferOptions(null)).toBeUndefined();
    expect(playerBufferOptions({ memoryClassMb: 0 })).toBeUndefined();
  });
});
