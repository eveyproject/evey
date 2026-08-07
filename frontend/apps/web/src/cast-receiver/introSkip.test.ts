import { describe, it, expect } from "vitest";
import { buildSegmentsUrl, parseSegments, planIntroSkip, introSkippedLabel } from "./introSkip";

const MASTER = "https://evey.example/api/v1/media/mi-1/hls/master.m3u8?token=tok-1&wasmAudio=1";

describe("buildSegmentsUrl", () => {
  it("targets the segments endpoint with the token only", () => {
    expect(buildSegmentsUrl(MASTER)).toBe(
      "https://evey.example/api/v1/media/mi-1/segments?types=intro&token=tok-1",
    );
  });

  it("returns null without a token or on a foreign URL", () => {
    expect(buildSegmentsUrl("https://evey.example/api/v1/media/mi-1/hls/master.m3u8")).toBeNull();
    expect(buildSegmentsUrl("https://cdn.example/video.mp4?token=t")).toBeNull();
  });
});

describe("parseSegments", () => {
  it("keeps plausible intro segments, in seconds", () => {
    expect(
      parseSegments({ segments: [{ type: "intro", startMs: 30_000, endMs: 90_000, confidence: 0.8, source: "detector" }] }),
    ).toEqual([{ type: "intro", startSec: 30, endSec: 90 }]);
  });

  it("drops non-intro types (a recap is content, credits belong to cast-next)", () => {
    expect(parseSegments({ segments: [{ type: "recap", startMs: 0, endMs: 30_000 }] })).toEqual([]);
    expect(parseSegments({ segments: [{ type: "end_credits", startMs: 0, endMs: 30_000 }] })).toEqual([]);
  });

  it("drops degenerate durations - an auto-jump cannot be undone from the couch", () => {
    expect(parseSegments({ segments: [{ type: "intro", startMs: 0, endMs: 3_000 }] })).toEqual([]);
    expect(parseSegments({ segments: [{ type: "intro", startMs: 0, endMs: 400_000 }] })).toEqual([]);
    expect(parseSegments({ segments: [{ type: "intro", startMs: "x", endMs: 90_000 }] })).toEqual([]);
  });

  it("tolerates malformed payloads", () => {
    expect(parseSegments(null)).toEqual([]);
    expect(parseSegments({})).toEqual([]);
    expect(parseSegments({ segments: "nope" })).toEqual([]);
  });
});

describe("planIntroSkip", () => {
  const segs = [{ type: "intro", startSec: 30, endSec: 90 }];

  it("fires inside the segment", () => {
    expect(planIntroSkip(segs, 31, new Set())).toEqual(segs[0]);
  });

  it("stays quiet outside and in the 1s tail", () => {
    expect(planIntroSkip(segs, 29, new Set())).toBeNull();
    expect(planIntroSkip(segs, 89.5, new Set())).toBeNull();
    expect(planIntroSkip(segs, 91, new Set())).toBeNull();
  });

  it("never re-skips a segment the viewer seeked back into", () => {
    expect(planIntroSkip(segs, 31, new Set([30]))).toBeNull();
  });
});

describe("introSkippedLabel", () => {
  it("localizes on the device language", () => {
    expect(introSkippedLabel("fr-FR")).toBe("Intro passée");
    expect(introSkippedLabel("en-US")).toBe("Intro skipped");
  });
});
