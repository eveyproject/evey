import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { WasmAudioEngineOptions } from "./WasmAudioEngine";
import { stallGateReportDue, useWasmAudioPlayback } from "./useWasmAudioPlayback";

// The web player's wasm-audio path reported NOTHING to the server before
// 2026-09-02: a beta tester's "no sound" had to be inferred from hls.js stall
// lines. This harness checks that the engine's mode, guard, fatal and
// stall-gate signals each reach the player-event reporter, and that the gate
// stream is thinned so it cannot drain the reporter's 20-event budget.

// The engine is replaced by a shell that only captures its options, so the
// test drives the callbacks by hand - no worker, no AudioContext.
let captured: WasmAudioEngineOptions | null = null;
const setUserOffsetSec = vi.fn();
vi.mock("./WasmAudioEngine", () => ({
  WasmAudioEngine: class {
    constructor(_video: HTMLVideoElement, _url: string, opts: WasmAudioEngineOptions) {
      captured = opts;
    }
    start = vi.fn(async () => {});
    destroy = vi.fn();
    setUserOffsetSec = setUserOffsetSec;
  },
}));

const MASTER_WASM = [
  "#EXTM3U",
  '#EVEY-AUDIO-URI:"od_audio1.m3u8?token=abc"',
  '#EVEY-AUDIO-AAC-URI:"audio_1.m3u8?token=abc"',
  '#EXT-X-STREAM-INF:BANDWIDTH=8000000,CODECS="avc1.640028"',
  "od_playlist.m3u8?token=abc",
].join("\n");

const MASTER_MUXED = ["#EXTM3U", '#EXT-X-STREAM-INF:BANDWIDTH=8000000,CODECS="avc1.640028,mp4a.40.2"', "v0.m3u8"].join("\n");

function mount(master: string, onStreamTokenRefresh = vi.fn(async () => "fresh-tok"), audioOffsetMs = 0) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(master, { status: 200 }));
  const video = document.createElement("video");
  Object.defineProperty(video, "currentTime", { value: 12.34, writable: true });
  const videoRef = { current: video };
  const streamTokenRef = { current: "tok" };
  const onStreamTokenRefreshRef = { current: onStreamTokenRefresh };
  const reportEvent = vi.fn();
  const onFallback = vi.fn();
  const hook = renderHook(
    (props: { audioOffsetMs: number }) =>
      useWasmAudioPlayback({
        videoRef,
        mediaId: "media-1",
        currentTrackIndex: 1,
        streamTokenRef,
        onStreamTokenRefresh: onStreamTokenRefreshRef,
        active: true,
        onFallback,
        reportEvent,
        audioOffsetMs: props.audioOffsetMs,
      }),
    { initialProps: { audioOffsetMs } },
  );
  return { reportEvent, onFallback, streamTokenRef, onStreamTokenRefresh, rerender: hook.rerender };
}

describe("useWasmAudioPlayback telemetry", () => {
  beforeEach(() => {
    captured = null;
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("forwards mode, guard and stall-gate signals with the picture's clock", async () => {
    const { reportEvent } = mount(MASTER_WASM);
    await waitFor(() => expect(captured).not.toBeNull());

    captured!.onModeChange!("aac");
    captured!.onDiagnostic!("probe refused: decode too slow");
    for (let cuts = 1; cuts <= 9; cuts++) captured!.onStallGate!(cuts);

    const types = reportEvent.mock.calls.map((c) => `${c[0].type}:${c[0].details}`);
    expect(types).toEqual([
      "wasm_audio_mode:aac",
      "wasm_audio_guard:probe refused: decode too slow",
      "wasm_audio_gate:cuts=1",
      "wasm_audio_gate:cuts=2",
      "wasm_audio_gate:cuts=4",
      "wasm_audio_gate:cuts=8",
    ]);
    expect(reportEvent.mock.calls[0][0]).toMatchObject({ posSec: 12.3, fatal: false });
  });

  it("reports a fatal engine error as fatal AND still falls back", async () => {
    const { reportEvent, onFallback } = mount(MASTER_WASM);
    await waitFor(() => expect(captured).not.toBeNull());

    captured!.onFatal!("wasm load failed");

    expect(reportEvent).toHaveBeenCalledWith(
      expect.objectContaining({ type: "wasm_audio_fatal", details: "wasm load failed", fatal: true }),
    );
    expect(onFallback).toHaveBeenCalledTimes(1);
  });

  it("wires onAuthError to the same token-refresh callback the video leg uses, and updates the shared ref", async () => {
    const { streamTokenRef, onStreamTokenRefresh } = mount(MASTER_WASM);
    await waitFor(() => expect(captured).not.toBeNull());

    const result = await captured!.onAuthError!();

    expect(onStreamTokenRefresh).toHaveBeenCalledTimes(1);
    expect(result).toBe("fresh-tok");
    // resolveUrl reads streamTokenRef - the refresh must land there, not just
    // be returned, or the very next segment fetch would reuse the stale token.
    expect(streamTokenRef.current).toBe("fresh-tok");
  });

  it("reports the server declining the wasm path before falling back", async () => {
    const { reportEvent, onFallback } = mount(MASTER_MUXED);
    await waitFor(() => expect(onFallback).toHaveBeenCalledTimes(1));

    expect(reportEvent).toHaveBeenCalledWith(expect.objectContaining({ type: "wasm_audio_mode", details: "declined" }));
    expect(captured).toBeNull();
  });
});

describe("stallGateReportDue", () => {
  it("hands the viewer's audio offset to the engine at creation and live on change, in seconds, audio-later positive", async () => {
    // -900 ms by ear on a Bluetooth/HDMI sink the browser under-reports: the
    // menu used to reach only the native AAC leg and do nothing here.
    const { rerender } = mount(MASTER_WASM, undefined, -900);
    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured!.userOffsetSec).toBeCloseTo(-0.9, 6);
    rerender({ audioOffsetMs: 250 });
    await waitFor(() => expect(setUserOffsetSec).toHaveBeenLastCalledWith(0.25));
  });

  it("passes the first cut and every doubling, nothing in between", () => {
    const due = Array.from({ length: 20 }, (_, i) => i + 1).filter(stallGateReportDue);
    expect(due).toEqual([1, 2, 4, 8, 16]);
    expect(stallGateReportDue(0)).toBe(false);
  });
});
