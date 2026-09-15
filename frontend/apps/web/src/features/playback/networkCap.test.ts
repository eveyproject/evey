import { describe, it, expect } from "vitest";
import { decideNetworkStep, estimateKbps, newNetworkCapState, observedRungKbps, recordStall, recordThroughput, shouldHandoffToTranscode } from "./networkCap";

const levels = [
  { height: 480, kbps: 1700 },
  { height: 720, kbps: 3700 },
  { height: 1080, kbps: 7200 },
  { height: 2160, kbps: 70000 },
];

// bytes per 3 s segment at a given line speed, transferred at that speed
function feed(s: ReturnType<typeof newNetworkCapState>, lineKbps: number, n: number, at = 0) {
  for (let i = 0; i < n; i++) {
    const bytes = (lineKbps * 1000 * 3) / 8;
    recordThroughput(s, { bytes, transferMs: 3000, at: at + i * 3000 });
  }
}

describe("networkCap", () => {
  it("needs three real samples before judging", () => {
    const s = newNetworkCapState();
    feed(s, 20000, 2);
    expect(estimateKbps(s)).toBeNull();
    recordThroughput(s, { bytes: 1000, transferMs: 3000, at: 9000 });
    expect(estimateKbps(s)).toBeNull();
    feed(s, 20000, 1, 9000);
    expect(estimateKbps(s)).toBeCloseTo(20000, -2);
  });

  it("steps down to the highest rung the line carries with headroom", () => {
    const s = newNetworkCapState();
    feed(s, 20000, 3);
    const step = decideNetworkStep(s, 30_000, levels, 3, 3, 40);
    expect(step).toMatchObject({ direction: "down", height: 1080 });
  });

  it("judges a rung on what its segments really weigh, not on the declared bitrate", () => {
    // Declared 70 Mb/s, segments shipped at 100 Mb/s over a 90 Mb/s line: the
    // declared figure says the line fits (90 > 70 x 1.2), the segments say no.
    const s = newNetworkCapState();
    const bytes = (100_000 * 1000 * 3) / 8;
    for (let i = 0; i < 3; i++) recordThroughput(s, { bytes, transferMs: (bytes * 8) / 90_000, at: i * 3000, durationSec: 3 });
    expect(observedRungKbps(s)).toBeCloseTo(100_000, -2);
    expect(decideNetworkStep(s, 30_000, levels, 3, 3, 4)).toMatchObject({ direction: "down", height: 1080 });
  });

  it("does not react to a rafale that was slow to produce", () => {
    // 3 s of content arriving in 200 ms of transfer = a fast line, whatever the wait was.
    const s = newNetworkCapState();
    for (let i = 0; i < 3; i++) recordThroughput(s, { bytes: 26_000_000, transferMs: 200, at: i * 3000 });
    expect(decideNetworkStep(s, 30_000, levels, 3, 3, 2)).toBeNull();
  });

  it("steps up one rung at a time with a full buffer and no stall", () => {
    const s = newNetworkCapState();
    feed(s, 20000, 3);
    expect(decideNetworkStep(s, 60_000, levels, 1, 3, 10)).toBeNull();
    const up = decideNetworkStep(s, 60_000, levels, 1, 3, 25);
    expect(up).toMatchObject({ direction: "up", height: 1080 });
    expect(decideNetworkStep(s, 70_000, levels, 2, 3, 25)).toBeNull();
    expect(decideNetworkStep(s, 100_000, levels, 2, 3, 25)).toBeNull();
  });

  it("never climbs past the render ceiling", () => {
    const s = newNetworkCapState();
    feed(s, 200000, 3);
    expect(decideNetworkStep(s, 60_000, levels, 2, 2, 40)).toBeNull();
  });

  it("two stalls on a line that only just fits step down early", () => {
    const s = newNetworkCapState();
    feed(s, 9000, 3);
    recordStall(s, 10_000);
    recordStall(s, 12_000);
    const step = decideNetworkStep(s, 30_000, levels, 2, 3, 1);
    expect(step).toMatchObject({ direction: "down", height: 720 });
  });

  it("respects the cooldown between changes", () => {
    const s = newNetworkCapState();
    feed(s, 2000, 3);
    expect(decideNetworkStep(s, 30_000, levels, 3, 3, 40)).not.toBeNull();
    expect(decideNetworkStep(s, 35_000, levels, 0, 3, 40)).toBeNull();
  });
});

describe("shouldHandoffToTranscode", () => {
  it("fires after sustained stalls on a single-variant master (no lower rung)", () => {
    const s = newNetworkCapState();
    recordStall(s, 1_000);
    recordStall(s, 2_000);
    expect(shouldHandoffToTranscode(s, 1, 2_500)).toBe(false); // 2 stalls: not yet
    recordStall(s, 3_000);
    expect(shouldHandoffToTranscode(s, 1, 3_500)).toBe(true); // 3 stalls → handoff
  });

  it("never fires when the master has a real ABR ladder (>1 level can step down)", () => {
    const s = newNetworkCapState();
    recordStall(s, 1_000);
    recordStall(s, 2_000);
    recordStall(s, 3_000);
    recordStall(s, 4_000);
    expect(shouldHandoffToTranscode(s, 3, 4_500)).toBe(false);
  });

  it("only counts stalls inside the sample window", () => {
    const s = newNetworkCapState();
    recordStall(s, 1_000);
    recordStall(s, 2_000);
    recordStall(s, 3_000);
    // 90s later the old stalls have aged out of the 60s window.
    expect(shouldHandoffToTranscode(s, 1, 93_000)).toBe(false);
  });
});
