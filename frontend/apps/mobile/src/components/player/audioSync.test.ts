import { describe, expect, it } from "vitest";
import { clampOffsetMs, formatOffsetMs, OFFSET_MAX_MS, stepOffsetMs, withAudioOffsetParam } from "./audioSync";

describe("clampOffsetMs", () => {
  it("passes values inside the range through untouched", () => {
    expect(clampOffsetMs(0)).toBe(0);
    expect(clampOffsetMs(1234)).toBe(1234);
    expect(clampOffsetMs(-1234)).toBe(-1234);
  });

  it("clamps at the +/-5s bounds", () => {
    expect(clampOffsetMs(999_999)).toBe(OFFSET_MAX_MS);
    expect(clampOffsetMs(-999_999)).toBe(-OFFSET_MAX_MS);
  });
});

describe("stepOffsetMs", () => {
  it("moves by 100ms per step", () => {
    expect(stepOffsetMs(0, 1)).toBe(100);
    expect(stepOffsetMs(100, -1)).toBe(0);
  });

  it("clamps a step that would cross the bound", () => {
    expect(stepOffsetMs(OFFSET_MAX_MS, 1)).toBe(OFFSET_MAX_MS);
    expect(stepOffsetMs(-OFFSET_MAX_MS, -1)).toBe(-OFFSET_MAX_MS);
  });
});

describe("formatOffsetMs", () => {
  it("formats zero", () => {
    expect(formatOffsetMs(0)).toBe("0 ms");
  });

  it("formats sub-second values in ms", () => {
    expect(formatOffsetMs(300)).toBe("+300 ms");
    expect(formatOffsetMs(-300)).toBe("-300 ms");
  });

  it("formats whole seconds without a decimal", () => {
    expect(formatOffsetMs(2000)).toBe("+2 s");
    expect(formatOffsetMs(-5000)).toBe("-5 s");
  });

  it("formats fractional seconds with one decimal", () => {
    expect(formatOffsetMs(1500)).toBe("+1.5 s");
  });
});

describe("withAudioOffsetParam", () => {
  it("returns the url unchanged for a zero offset", () => {
    expect(withAudioOffsetParam("https://h/master.m3u8?token=t", 0)).toBe(
      "https://h/master.m3u8?token=t",
    );
  });

  it("returns an empty url unchanged", () => {
    expect(withAudioOffsetParam("", 300)).toBe("");
  });

  it("appends after an existing query string", () => {
    expect(withAudioOffsetParam("https://h/master.m3u8?token=t", 300)).toBe(
      "https://h/master.m3u8?token=t&audioOffsetMs=300",
    );
  });

  it("starts a query string when the url has none", () => {
    expect(withAudioOffsetParam("https://h/master.m3u8", 300)).toBe(
      "https://h/master.m3u8?audioOffsetMs=300",
    );
  });

  it("truncates a fractional ms value", () => {
    expect(withAudioOffsetParam("https://h/master.m3u8?token=t", 300.7)).toBe(
      "https://h/master.m3u8?token=t&audioOffsetMs=300",
    );
  });

  it("preserves a fragment placed after the query", () => {
    expect(withAudioOffsetParam("https://h/master.m3u8?token=t#frag", 300)).toBe(
      "https://h/master.m3u8?token=t&audioOffsetMs=300#frag",
    );
  });
});
