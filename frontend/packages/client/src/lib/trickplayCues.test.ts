import { describe, it, expect } from "vitest";
import { parseTrickplayVTT, cueForTime, type TrickplayCue } from "./trickplayCues";

const REALISTIC_VTT = `WEBVTT

00:00:00.000 --> 00:00:04.500
/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc#xywh=0,0,160,90

00:00:04.500 --> 00:00:09.200
/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc#xywh=160,0,160,90

00:00:09.200 --> 00:00:13.800
/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc#xywh=320,0,160,90
`;

describe("parseTrickplayVTT", () => {
  it("parses a realistic manifest into cues with time, url and tile rect", () => {
    const cues = parseTrickplayVTT(REALISTIC_VTT);
    expect(cues).toHaveLength(3);
    expect(cues[0]).toEqual({
      startTime: 0,
      endTime: 4.5,
      url: "/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc",
      x: 0,
      y: 0,
      w: 160,
      h: 90,
    });
    expect(cues[1]).toEqual({
      startTime: 4.5,
      endTime: 9.2,
      url: "/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc",
      x: 160,
      y: 0,
      w: 160,
      h: 90,
    });
  });

  it("keeps the ?token= query on the url and only strips the #xywh fragment", () => {
    const cues = parseTrickplayVTT(REALISTIC_VTT);
    for (const cue of cues) {
      expect(cue.url).toContain("?token=abc");
      expect(cue.url).not.toContain("#xywh");
    }
  });

  it("returns an empty array for an empty manifest (no trickplay generated yet)", () => {
    expect(parseTrickplayVTT("WEBVTT\n")).toEqual([]);
  });

  it("skips a cue block missing its tile rect instead of throwing", () => {
    const noXywh = `WEBVTT

00:00:00.000 --> 00:00:04.500
/api/v1/media/mi-1/trickplay/trickplay_001.webp?token=abc
`;
    expect(parseTrickplayVTT(noXywh)).toEqual([]);
  });
});

describe("cueForTime", () => {
  const cues: TrickplayCue[] = [
    { startTime: 0, endTime: 4.5, url: "a", x: 0, y: 0, w: 160, h: 90 },
    { startTime: 4.5, endTime: 9.2, url: "a", x: 160, y: 0, w: 160, h: 90 },
    { startTime: 9.2, endTime: 13.8, url: "a", x: 320, y: 0, w: 160, h: 90 },
  ];

  it("returns null before the first cue", () => {
    expect(cueForTime(cues, -1)).toBeNull();
  });

  it("returns null after the last cue", () => {
    expect(cueForTime(cues, 20)).toBeNull();
  });

  it("resolves a time inside the middle cue", () => {
    expect(cueForTime(cues, 7)).toEqual(cues[1]);
  });

  it("resolves a boundary time to the cue that starts there, not the one that ends there", () => {
    expect(cueForTime(cues, 4.5)).toEqual(cues[1]);
  });

  it("returns null for an empty cue list (no trickplay for this media)", () => {
    expect(cueForTime([], 5)).toBeNull();
  });
});
