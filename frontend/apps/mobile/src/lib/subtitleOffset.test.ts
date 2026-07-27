import { describe, expect, it } from "vitest";
import { applyCueOffset, clampSubtitleOffsetMs, SUBTITLE_OFFSET_MAX_MS } from "./subtitleOffset";

describe("clampSubtitleOffsetMs", () => {
  it("passes values inside the range through untouched", () => {
    expect(clampSubtitleOffsetMs(0)).toBe(0);
    expect(clampSubtitleOffsetMs(2500)).toBe(2500);
  });

  it("clamps at the +/-10s bounds", () => {
    expect(clampSubtitleOffsetMs(999_999)).toBe(SUBTITLE_OFFSET_MAX_MS);
    expect(clampSubtitleOffsetMs(-999_999)).toBe(-SUBTITLE_OFFSET_MAX_MS);
  });
});

describe("applyCueOffset", () => {
  it("shifts start and end by the offset", () => {
    const out = applyCueOffset([{ start: 10, end: 12 }], 2);
    expect(out).toEqual([{ start: 12, end: 14 }]);
  });

  it("shifts negative and clamps the start at zero", () => {
    const out = applyCueOffset([{ start: 1, end: 3 }], -5);
    expect(out).toEqual([{ start: 0, end: 0 }]);
  });

  it("keeps end >= start when only the start clamps", () => {
    const out = applyCueOffset([{ start: 1, end: 10 }], -5);
    // end shifts to 5, start clamps to 0 - end stays 5, not pulled down to start.
    expect(out).toEqual([{ start: 0, end: 5 }]);
  });

  it("is a pure function - does not mutate the input array", () => {
    const input = [{ start: 10, end: 12 }];
    const out = applyCueOffset(input, 5);
    expect(input).toEqual([{ start: 10, end: 12 }]);
    expect(out).not.toBe(input);
  });
});
