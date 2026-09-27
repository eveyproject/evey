import { describe, expect, it } from "vitest";
import { pcmIsSilent, SilenceRun } from "./silenceWatch";

const planar = (...planes: number[][]) => ({
  kind: "pcm" as const,
  channels: planes.length,
  sampleRate: 48000,
  frames: planes[0].length,
  planar: planes.map((p) => new Float32Array(p)),
});

describe("pcmIsSilent", () => {
  it("is true only when every sample of every plane is zero", () => {
    expect(pcmIsSilent(planar([0, 0, 0], [0, 0, 0]))).toBe(true);
    expect(pcmIsSilent(planar([0, 0, 0], [0, 1e-6, 0]))).toBe(false);
  });

  it("reads the interleaved shape of an old cached worker too", () => {
    const d = { kind: "pcm" as const, channels: 2, sampleRate: 48000, frames: 2, pcm: new Float32Array([0, 0, 0, 0]) };
    expect(pcmIsSilent(d)).toBe(true);
    d.pcm[3] = -0.5;
    expect(pcmIsSilent(d)).toBe(false);
  });
});

describe("SilenceRun", () => {
  it("alarms once, when consecutive silent segments reach the threshold", () => {
    const run = new SilenceRun(10);
    expect(run.record(0, 3, true)).toBe(false);
    expect(run.record(1, 3, true)).toBe(false);
    expect(run.record(2, 3, true)).toBe(false);
    expect(run.record(3, 3, true)).toBe(true);
    expect(run.record(4, 3, true)).toBe(false);
    expect(run.silentSec).toBe(15);
    expect(run.segments).toBe(5);
  });

  it("restarts the run on a heard segment or a jump in the index", () => {
    const run = new SilenceRun(10);
    run.record(0, 6, true);
    run.record(1, 6, false);
    expect(run.record(2, 6, true)).toBe(false);
    // A seek: segment 40 does not follow 2, the 6 s before it do not count.
    expect(run.record(40, 6, true)).toBe(false);
    expect(run.currentRunSec).toBe(6);
    expect(run.record(41, 6, true)).toBe(true);
  });
});
