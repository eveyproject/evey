import { describe, expect, it } from "vitest";
import { findActiveCueIndex, parseVttCues, shiftCues } from "./subtitleCues";

const VTT = `WEBVTT

1
00:00:01.000 --> 00:00:03.500
Hello <b>world</b>

00:00:04.000 --> 00:00:06.000
Line one
Line two

01:00:00.000 --> 01:00:02.000
Hour cue
`;

describe("parseVttCues", () => {
  it("parses cues with start/end/text, sorted by start", () => {
    const cues = parseVttCues(VTT);
    expect(cues).toHaveLength(3);
    expect(cues[0]).toEqual({ start: 1, end: 3.5, text: "Hello world" });
    expect(cues[1].text).toBe("Line one\nLine two");
    expect(cues[2].start).toBe(3600);
  });

  it("strips inline markup tags", () => {
    expect(parseVttCues(VTT)[0].text).not.toContain("<b>");
  });

  it("skips a leading cue identifier line", () => {
    expect(parseVttCues(VTT)[0].text).toBe("Hello world");
  });

  it("returns an empty list for a header-only VTT", () => {
    expect(parseVttCues("WEBVTT\n\n")).toEqual([]);
  });

  it("drops cue settings after the timing line", () => {
    const vtt = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000 align:start line:0\nHi\n";
    const cues = parseVttCues(vtt);
    expect(cues).toEqual([{ start: 1, end: 2, text: "Hi" }]);
  });
});

describe("shiftCues", () => {
  it("returns the same array reference when offset is zero", () => {
    const cues = parseVttCues(VTT);
    expect(shiftCues(cues, 0)).toBe(cues);
  });

  it("shifts start/end and keeps the text", () => {
    const cues = [{ start: 10, end: 12, text: "hi" }];
    expect(shiftCues(cues, 2000)).toEqual([{ start: 12, end: 14, text: "hi" }]);
  });

  it("clamps at zero like applyCueOffset", () => {
    const cues = [{ start: 1, end: 3, text: "hi" }];
    expect(shiftCues(cues, -5000)).toEqual([{ start: 0, end: 0, text: "hi" }]);
  });
});

describe("findActiveCueIndex", () => {
  const cues = [
    { start: 1, end: 3, text: "a" },
    { start: 5, end: 8, text: "b" },
    { start: 8, end: 10, text: "c" },
  ];

  it("returns -1 before the first cue", () => {
    expect(findActiveCueIndex(cues, 0)).toBe(-1);
  });

  it("returns the cue index within its range", () => {
    expect(findActiveCueIndex(cues, 2)).toBe(0);
    expect(findActiveCueIndex(cues, 6)).toBe(1);
  });

  it("returns -1 in the gap between cues", () => {
    expect(findActiveCueIndex(cues, 4)).toBe(-1);
  });

  it("is inclusive of both boundaries", () => {
    expect(findActiveCueIndex(cues, 1)).toBe(0);
    expect(findActiveCueIndex(cues, 3)).toBe(0);
  });

  it("picks the later cue at a shared boundary", () => {
    expect(findActiveCueIndex(cues, 8)).toBe(2);
  });

  it("returns -1 after the last cue", () => {
    expect(findActiveCueIndex(cues, 100)).toBe(-1);
  });

  it("returns -1 for an empty cue list", () => {
    expect(findActiveCueIndex([], 5)).toBe(-1);
  });
});
