import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { DecoderStats } from "@evey/media-caps";

vi.mock("@evey/media-caps", async () => {
  const real = await vi.importActual<typeof import("../../../media-caps/src/decoderStats")>(
    "../../../media-caps/src/decoderStats",
  );
  const guard = await vi.importActual<typeof import("../../../media-caps/src/decoderGuard")>(
    "../../../media-caps/src/decoderGuard",
  );
  return {
    ...real,
    createDecoderWatchdog: guard.createDecoderWatchdog,
    getPlaybackDecoderStats: vi.fn(),
    resetPlaybackDecoderStats: vi.fn(),
    disablePlaybackDecoder: vi.fn(),
    markPlaybackDecoderIdle: native.idle,
    setPlaybackDecoderMedia: vi.fn(),
  };
});

const native = vi.hoisted(() => ({
  idle: vi.fn(),
  appState: null as ((s: string) => void) | null,
}));
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_e: string, cb: (s: string) => void) => {
      native.appState = cb;
      return { remove: () => (native.appState = null) };
    },
  },
}));

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
  decodeMs: 0,
  maxDecodeMs: 0,
  underruns: 0,
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
    const { unmount } = renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset }));
    expect(reset).toHaveBeenCalledTimes(1);
    await tick(10_000);
    unmount();
    await tick(0);
    expect(report).not.toHaveBeenCalled();
  });

  // The silent E-AC-3 case: the picture plays, ac3go hands back exact zeros.
  it("reports the stats once ten seconds are in, the silence once, and the run on close", async () => {
    const { unmount } = renderHook(() => useDecoderTelemetry("m1", report, () => 42.4, { readStats: read, resetStats: reset }));
    current = stats({ frames: 200 });
    await tick(2_000);
    expect(report).not.toHaveBeenCalled();

    current = stats({ frames: 400, silentFrames: 400, silentRunFrames: 400 });
    await tick(2_000);
    expect(report.mock.calls.map((c) => c[0].type)).toEqual(["ac3go_stats", "ac3go_silence_error"]);
    expect(report.mock.calls[1][0]).toMatchObject({
      details: "codec=eac3 src=6ch out=2ch max=2ch rate=48000 frames=400 bad=0 silent=400 run=400 rtf=0.000 dmax=0.0ms ur=0",
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
    renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset }));
    current = stats({ frames: 2000, silentFrames: 5, silentRunFrames: 0 });
    await tick(6_000);
    expect(report.mock.calls.map((c) => c[0].type)).toEqual(["ac3go_stats"]);
  });

  // A trace of the run while it lasts: the app may die before the closing beacon.
  it("reports the stats again every minute", async () => {
    renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset }));
    current = stats({ frames: 2000 });
    await tick(2_000);
    await tick(58_000);
    expect(report).toHaveBeenCalledTimes(1);
    await tick(2_000);
    expect(report.mock.calls.map((c) => c[0].type)).toEqual(["ac3go_stats", "ac3go_stats"]);
  });

  // 2026-10-01, a Chromecast HD: ac3go fell behind, the picture crawled, the app stopped.
  it("drops ac3go and asks for a restart once it is behind for two windows", async () => {
    const onTooSlow = vi.fn();
    const disable = vi.fn();
    renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset, onTooSlow, disable }));
    // 125 frames = 4 s of audio per poll; 3.2 s of decoding each: rtf 0.8.
    let frames = 0;
    let ms = 0;
    for (let i = 0; i < 3; i++) {
      frames += 125;
      ms += 3200;
      current = stats({ frames, decodeMs: ms });
      await tick(2_000);
    }
    expect(disable).toHaveBeenCalledTimes(1);
    expect(disable.mock.calls[0][0]).toBe("rtf=0.80/ur=0,rtf=0.80/ur=0");
    expect(onTooSlow).toHaveBeenCalledTimes(1);
    expect(report.mock.calls.map((c) => c[0].type)).toContain("ac3go_too_slow_error");

    // Once per playback.
    frames += 125;
    ms += 3200;
    current = stats({ frames, decodeMs: ms });
    await tick(2_000);
    expect(onTooSlow).toHaveBeenCalledTimes(1);
  });

  it("leaves a decoder that keeps up alone", async () => {
    const onTooSlow = vi.fn();
    renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset, onTooSlow, disable: vi.fn() }));
    for (let i = 1; i <= 6; i++) {
      current = stats({ frames: 125 * i, decodeMs: 400 * i });
      await tick(2_000);
    }
    expect(onTooSlow).not.toHaveBeenCalled();
  });

  // Leaving the screen is not a crash: an OS kill in the background must not
  // be blamed on ac3go at the next launch.
  it("tells the native guard when the app leaves the screen", async () => {
    const { unmount } = renderHook(() => useDecoderTelemetry("m1", report, () => 0, { readStats: read, resetStats: reset }));
    native.appState?.("background");
    expect(native.idle).toHaveBeenCalledTimes(1);
    unmount();
    expect(native.appState).toBeNull();
  });

  it("does nothing without a media id", async () => {
    renderHook(() => useDecoderTelemetry("", report, () => 0, { readStats: read, resetStats: reset }));
    current = stats({ frames: 2000, silentRunFrames: 2000 });
    await tick(4_000);
    expect(reset).not.toHaveBeenCalled();
    expect(report).not.toHaveBeenCalled();
  });
});
