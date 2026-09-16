import { describe, it, expect } from "vitest";
import { applyCuePosition, cueFontSizePx } from "./useSubtitleManager";

function fakeCue(line: number | "auto" = "auto", snapToLines = true) {
  return { line, snapToLines } as unknown as VTTCue;
}

function fakeVideo(tracks: Array<{ mode: TextTrackMode; cues: VTTCue[] }>): HTMLVideoElement {
  return { textTracks: tracks } as unknown as HTMLVideoElement;
}

describe("applyCuePosition", () => {
  it("anchors bottom cues at 88% with percentage lines (not glued to the edge)", () => {
    const c1 = fakeCue();
    const c2 = fakeCue(100, true); // the old buggy state
    const video = fakeVideo([{ mode: "showing", cues: [c1, c2] }]);

    applyCuePosition(video, "bottom");

    for (const c of [c1, c2]) {
      expect(c.snapToLines).toBe(false); // percentage interpretation
      expect(c.line).toBe(88);
    }
  });

  it("anchors top cues at 8%", () => {
    const c = fakeCue();
    const video = fakeVideo([{ mode: "showing", cues: [c] }]);
    applyCuePosition(video, "top");
    expect(c.line).toBe(8);
    expect(c.snapToLines).toBe(false);
  });

  it("keeps a cue the file places in the upper half (ASS {\\an8} rendered as line:8%) under the bottom preference", () => {
    const sign = fakeCue(8, false); // authored percentage line, upper half
    const dialogue = fakeCue();
    const video = fakeVideo([{ mode: "showing", cues: [sign, dialogue] }]);

    applyCuePosition(video, "bottom");
    expect(sign.line).toBe(8);
    expect(dialogue.line).toBe(88);

    // Applied again after our own pass: the authored placement is remembered,
    // not re-derived from the line we just wrote.
    applyCuePosition(video, "bottom");
    expect(sign.line).toBe(8);

    // The viewer asking for everything at the top wins over the author.
    applyCuePosition(video, "top");
    expect(sign.line).toBe(8);
    expect(dialogue.line).toBe(8);
  });

  it("leaves hidden tracks untouched", () => {
    const shown = fakeCue();
    const hidden = fakeCue(42, true);
    const video = fakeVideo([
      { mode: "showing", cues: [shown] },
      { mode: "hidden", cues: [hidden] },
    ]);
    applyCuePosition(video, "bottom");
    expect(shown.line).toBe(88);
    expect(hidden.line).toBe(42);
    expect(hidden.snapToLines).toBe(true);
  });
});

describe("cueFontSizePx", () => {
  it("reproduces the native 5%-of-height size at fontSize=100", () => {
    expect(cueFontSizePx(1000, 100)).toBe(50);
    expect(cueFontSizePx(720, 100)).toBe(36);
  });

  it("scales with the user preference", () => {
    expect(cueFontSizePx(1000, 50)).toBe(25);
    expect(cueFontSizePx(1000, 150)).toBe(75);
  });

  it("clamps to a readable minimum on tiny players", () => {
    expect(cueFontSizePx(200, 50)).toBe(13);
  });

  it("returns 0 when the element has no measurable height", () => {
    expect(cueFontSizePx(0, 100)).toBe(0);
  });
});

describe("applySubtitleOffset", () => {
  function videoWithCue(start: number, end: number) {
    const video = document.createElement("video");
    const cue = { startTime: start, endTime: end, line: 90 } as VTTCue;
    const track = { mode: "showing", cues: [cue] } as unknown as TextTrack;
    Object.defineProperty(video, "textTracks", {
      value: { length: 1, 0: track },
    });
    return { video, cue };
  }

  it("shifts loaded cues in place, preserving duration", async () => {
    const { applySubtitleOffset } = await import("./useSubtitleManager");
    const { video, cue } = videoWithCue(10, 12.5);
    applySubtitleOffset(video, 0.3, new WeakMap());
    expect(cue.startTime).toBeCloseTo(10.3);
    expect(cue.endTime).toBeCloseTo(12.8);
  });

  it("clamps a cue at zero without collapsing it", async () => {
    const { applySubtitleOffset } = await import("./useSubtitleManager");
    const { video, cue } = videoWithCue(0.2, 2.2);
    applySubtitleOffset(video, -1, new WeakMap());
    expect(cue.startTime).toBe(0);
    expect(cue.endTime).toBeCloseTo(1.2);
  });

  // Clamping must not LOSE the original timing: shifting a head cue past zero
  // and then resetting the offset has to land back on the source times
  // exactly (the naive delta approach drifted by the clamped amount).
  it("is exactly reversible across a zero clamp", async () => {
    const { applySubtitleOffset } = await import("./useSubtitleManager");
    const { video, cue } = videoWithCue(0.2, 2.2);
    const base = new WeakMap();
    applySubtitleOffset(video, -1, base);
    expect(cue.startTime).toBe(0);
    applySubtitleOffset(video, 0, base);
    expect(cue.startTime).toBeCloseTo(0.2);
    expect(cue.endTime).toBeCloseTo(2.2);
  });
});
