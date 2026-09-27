import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { DecoderStats } from "@evey/media-caps";

vi.mock("@evey/media-caps", async () => {
  const real = await vi.importActual<typeof import("../../../media-caps/src/decoderStats")>(
    "../../../media-caps/src/decoderStats",
  );
  return { ...real, getPlaybackDecoderStats: vi.fn(), resetPlaybackDecoderStats: vi.fn() };
});

import { useDecoderTelemetry } from "./useDecoderTelemetry";

const stats = (over: Partial<DecoderStats>): DecoderStats => ({
  codec: "eac3",
  sourceChannels: 6,
  outputChannels: 2,
  maxChannels: 2,
  sampleRate: 48000,
  frames: 0,
  badFrames: 0,
  silentFrames: 0,
  silentRunFrames: 0,
  decoder: "",
  ...over,
});

let current: DecoderStats | null;
const read = vi.fn(async () => current);
const reset = vi.fn(async () => {});
const report = vi.fn();

async function tick(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  current = null;
  read.mockClear();
  reset.mockClear();
  report.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useDecoderTelemetry", () => {
  it("stays quiet when ac3go never took a track, and forgets the previous title on mount", async () => {
    const { unmount } = renderHook(() => useDecoderTelemetry("m1", report, () => 0, read, reset));
    expect(reset).toHaveBeenCalledTimes(1);
    await tick(10_000);
    unmount();
    await tick(0);
    expect(report).not.toHaveBeenCalled();
  });

  // The silent E-AC-3 case: the picture plays, ac3go hands back exact zeros.
  it("reports the stats once ten seconds are in, the silence once, and the run on close", async () => {
    const { unmount } = renderHook(() => useDecoderTelemetry("m1", report, () => 42.4, read, reset));
    current = stats({ frames: 200 });
    await tick(2_000);
    expect(report).not.toHaveBeenCalled();

    current = stats({ frames: 400, silentFrames: 400, silentRunFrames: 400 });
    await tick(2_000);
    expect(report.mock.calls.map((c) => c[0].type)).toEqual(["ac3go_stats", "ac3go_silence_error"]);
    expect(report.mock.calls[1][0]).toMatchObject({
      details: "codec=eac3 src=6ch out=2ch max=2ch rate=48000 frames=400 bad=0 silent=400 run=400",
      fatal: false,
      posSec: 42,
    });

    current = stats({ frames: 600, silentFrames: 600, silentRunFrames: 600 });
    await tick(4_000);
    expect(report).toHaveBeenCalledTimes(2);

    unmount();
    await tick(0);
    expect(report).toHaveBeenCalledTimes(3);
    expect(report.mock.calls[2][0]).toMatchObject({ type: "ac3go_stats_end", details: expect.stringContaining("frames=600") });
  });

  it("does not alarm on a heard track", async () => {
    renderHook(() => useDecoderTelemetry("m1", report, () => 0, read, reset));
    current = stats({ frames: 2000, silentFrames: 5, silentRunFrames: 0 });
    await tick(6_000);
    expect(report.mock.calls.map((c) => c[0].type)).toEqual(["ac3go_stats"]);
  });

  it("does nothing without a media id", async () => {
    renderHook(() => useDecoderTelemetry("", report, () => 0, read, reset));
    current = stats({ frames: 2000, silentRunFrames: 2000 });
    await tick(4_000);
    expect(reset).not.toHaveBeenCalled();
    expect(report).not.toHaveBeenCalled();
  });
});
