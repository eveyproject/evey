import { describe, it, expect } from "vitest";
import { playbackSnapshot, registerEngineStats } from "./playbackSnapshot";

function fakeVideo(opts: { ct: number; ranges: [number, number][]; readyState?: number; dropped?: number; total?: number }): HTMLVideoElement {
  const ranges = opts.ranges;
  return {
    currentTime: opts.ct,
    readyState: opts.readyState ?? 4,
    playbackRate: 1,
    buffered: {
      length: ranges.length,
      start: (i: number) => ranges[i][0],
      end: (i: number) => ranges[i][1],
    },
    getVideoPlaybackQuality: () => ({ droppedVideoFrames: opts.dropped ?? 0, totalVideoFrames: opts.total ?? 0 }),
  } as unknown as HTMLVideoElement;
}

describe("playbackSnapshot", () => {
  it("tells a full buffer from an empty one at the playhead", () => {
    // The stall that mattered on 2026-09-03: 18 s buffered, picture frozen.
    const full = playbackSnapshot(fakeVideo({ ct: 329.1, ranges: [[320, 347.2]], dropped: 12, total: 2300 }), null);
    expect(full).toContain("buf=320.0-347.2");
    expect(full).toContain("drop=12/2300");
    expect(full).toContain("rs=4");

    const empty = playbackSnapshot(fakeVideo({ ct: 329.1, ranges: [[300, 320]], readyState: 2 }), null);
    expect(empty).toContain("buf=none");
    expect(empty).toContain("rs=2");
  });

  it("measures the hole to the next buffered range", () => {
    const s = playbackSnapshot(fakeVideo({ ct: 10, ranges: [[0, 12], [12.4, 30]] }), null);
    expect(s).toContain("buf=0.0-12.0 next+0.4");
  });

  it("carries the wasm audio leg's state once the engine registers it", () => {
    const video = fakeVideo({ ct: 5, ranges: [[0, 20]] });
    const unregister = registerEngineStats(video, () => ({ mode: "wasm", ctxState: "suspended", anchored: false, activeSources: 0, decoded: 3, driftMs: 41.6, leadMs: 100, leadMeasured: false, outputLatencyMs: 32.8 }));
    // lead= is the delay the schedule adds; the star marks the static base, not a measurement.
    expect(playbackSnapshot(video)).toContain("wasm=wasm/suspended/free/src0/dec3 drift=42ms lead=100ms* out=33ms");
    unregister();
    expect(playbackSnapshot(video)).not.toContain("wasm=");
  });
});
