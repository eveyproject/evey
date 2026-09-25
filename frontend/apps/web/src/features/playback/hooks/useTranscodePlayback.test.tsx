import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import React from "react";

// hls.js is replaced by a recorder: the contracts under test are which
// events the hook reacts to and what it drives on the instance in return.
const H = vi.hoisted(() => {
  class FakeHls {
    static Events = { LEVEL_SWITCHED: "LEVEL_SWITCHED", MANIFEST_PARSED: "MANIFEST_PARSED", ERROR: "ERROR", FRAG_LOADED: "FRAG_LOADED" };
    static ErrorTypes = { NETWORK_ERROR: "networkError", MEDIA_ERROR: "mediaError", OTHER_ERROR: "otherError" };
    static instances: FakeHls[] = [];
    handlers = new Map<string, (evt: string, data: any) => void>();
    config: Record<string, unknown>;
    levels: { height: number; bitrate: number }[] = [{ height: 480, bitrate: 1e6 }, { height: 1080, bitrate: 8e6 }];
    audioTracks: { name?: string; lang?: string }[] = [{ name: "French", lang: "fr" }, { name: "English", lang: "en" }];
    audioTrack = -1;
    currentLevel = -1;
    startLevel = -1;
    loadSource = vi.fn();
    attachMedia = vi.fn();
    destroy = vi.fn();
    startLoad = vi.fn();
    recoverMediaError = vi.fn();
    constructor(config: Record<string, unknown>) {
      this.config = config;
      FakeHls.instances.push(this);
    }
    on(evt: string, cb: (evt: string, data: any) => void) {
      this.handlers.set(evt, cb);
    }
    emit(evt: string, data: any = {}) {
      this.handlers.get(evt)?.(evt, data);
    }
  }
  return { FakeHls };
});
vi.mock("hls.js", () => ({ default: H.FakeHls }));

const utils = vi.hoisted(() => ({
  buildUrl: vi.fn((...a: unknown[]) => `hls://${a.join("|")}`),
  native: false,
  startPlayback: vi.fn(async () => {}),
  liftMute: vi.fn(() => vi.fn()),
}));
vi.mock("./hlsPlayerUtils", () => ({
  buildTranscodeHlsUrl: (...a: unknown[]) => utils.buildUrl(...a),
  prefersNativeHLS: () => utils.native,
  startPlayback: (...a: unknown[]) => utils.startPlayback(...(a as [])),
  liftForcedMuteOnNextGesture: (...a: unknown[]) => utils.liftMute(...(a as [])),
  pickPinnedLevel: ({ levelHeights, desiredHeight }: { levelHeights: number[]; desiredHeight: number }) =>
    desiredHeight ? levelHeights.indexOf(desiredHeight) : levelHeights.length - 1,
  pickTranscodeResumeSec: (pos: number, start: number) => (pos > 0 ? pos : start),
  forwardBufferSeconds: () => 12,
  MAX_FORWARD_BUFFER_SEC: 30,
}));
const report = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ createPlayerEventReporter: () => report }));
vi.mock("../playbackSnapshot", () => ({ playbackSnapshot: () => "snap" }));

import { useTranscodePlayback } from "./useTranscodePlayback";

function setup(over: Record<string, unknown> = {}) {
  const video = document.createElement("video");
  Object.defineProperty(video, "currentTime", { value: 0, writable: true });
  Object.defineProperty(video, "paused", { value: true, writable: true });
  Object.defineProperty(video, "muted", { value: false, writable: true });
  video.load = vi.fn();
  const refs = {
    videoRef: { current: video },
    hlsRef: { current: null as any },
    positionRef: { current: 0 },
    transcodeOffsetRef: { current: 0 },
    wasPausedRef: { current: false },
    streamTokenRef: { current: "tok1" },
    onStreamTokenRefreshRef: { current: vi.fn(async () => "tok2") },
    pinnedQualityRef: { current: 0 },
    audioTrackOverrideRef: { current: null as number | null },
    onBufferStallRef: { current: vi.fn() },
  };
  const spies = { setIsBuffering: vi.fn(), setFatalError: vi.fn(), setShedReason: vi.fn(), onEnded: vi.fn() };
  const args = {
    ...refs, ...spies,
    transcodeMode: true, mediaId: "m1", currentTrackIndex: 0, selectedBurnInTrack: null, startPositionMs: 0,
    ...over,
  };
  const hook = renderHook((p: any) => useTranscodePlayback(p), { initialProps: args });
  const hls = () => H.FakeHls.instances.at(-1)!;
  return { video, refs, spies, hook, hls, args };
}

const net503 = (details: string, extra: Record<string, unknown> = {}) => ({
  fatal: true, type: "networkError", details, response: { code: 503, text: "Service Unavailable" }, ...extra,
});

beforeEach(() => {
  H.FakeHls.instances.length = 0;
  utils.native = false;
  utils.buildUrl.mockClear();
  utils.startPlayback.mockClear();
  report.mockClear();
});
afterEach(() => vi.useRealTimers());

describe("useTranscodePlayback - start", () => {
  // The instance loads from the resume point (no fragment 0 nobody reads),
  // the rung is pinned before the first fragment, the manifest's audio
  // renditions are surfaced, and a rebuild while paused must not resume.
  it("builds the URL, pins the top rung on MANIFEST_PARSED and resumes at the live playhead", async () => {
    const { video, refs, hook, hls } = setup({ startPositionMs: 0 });
    refs.positionRef.current = 0;
    expect(utils.buildUrl).toHaveBeenCalledWith("m1", "tok1", 0, null, 0, 0, false, false, false);
    expect(hls().config.startPosition).toBe(-1);
    expect(hls().config.testBandwidth).toBe(false);
    expect(hls().loadSource).toHaveBeenCalledWith("hls://m1|tok1|0||0|0|false|false|false");
    expect(hls().attachMedia).toHaveBeenCalledWith(video);
    expect(refs.hlsRef.current).toBe(hls());

    await act(async () => hls().emit("MANIFEST_PARSED"));
    expect(hls().startLevel).toBe(1);
    expect(hls().currentLevel).toBe(1);
    expect(hls().config.maxBufferLength).toBe(12);
    expect(hook.result.current.hlsLevels).toEqual([480, 1080]);
    expect(hook.result.current.manifestAudioTracks).toEqual([{ name: "French", lang: "fr" }, { name: "English", lang: "en" }]);
    expect(utils.startPlayback).toHaveBeenCalledTimes(1);

    await act(async () => hls().emit("LEVEL_SWITCHED", { level: 0 }));
    expect(hook.result.current.autoLevelHeight).toBe(480);
  });

  it("honours a manual rung, a client-side audio switch, and does not resume while paused", async () => {
    const { refs, hls } = setup();
    refs.pinnedQualityRef.current = 480;
    refs.audioTrackOverrideRef.current = 1;
    refs.wasPausedRef.current = true;
    await act(async () => hls().emit("MANIFEST_PARSED"));
    expect(hls().currentLevel).toBe(0);
    expect(hls().audioTrack).toBe(1);
    expect(utils.startPlayback).not.toHaveBeenCalled();
  });

  it("arms the forced-mute recovery when the browser muted a viewer who wanted sound", async () => {
    const { video, hls } = setup();
    utils.startPlayback.mockImplementationOnce(async () => {
      (video as any).muted = true;
    });
    await act(async () => hls().emit("MANIFEST_PARSED"));
    expect(utils.liftMute).toHaveBeenCalledWith(video);
  });

  it("a paused player is never buffering, and the teardown destroys the instance", () => {
    const { video, spies, hook, hls, refs } = setup();
    act(() => { video.dispatchEvent(new Event("waiting")); });
    expect(spies.setIsBuffering).not.toHaveBeenCalledWith(true);
    (video as any).paused = false;
    act(() => { video.dispatchEvent(new Event("waiting")); });
    expect(spies.setIsBuffering).toHaveBeenLastCalledWith(true);
    act(() => { video.dispatchEvent(new Event("ended")); });
    expect(spies.onEnded).toHaveBeenCalledTimes(1);
    const first = hls();
    hook.unmount();
    expect(first.destroy).toHaveBeenCalledTimes(1);
    expect(refs.hlsRef.current).toBeNull();
    expect(refs.wasPausedRef.current).toBe(false);
  });
});

describe("useTranscodePlayback - errors", () => {
  // A 503 is a typed shed only when the server says so (header first, then
  // the JSON body); a bare 503 on stream start is framed as "too many
  // streams"; a bare 503 on a FRAGMENT is the segment handler's "not yet
  // ready" and goes through the retry backoff instead of a shed overlay.
  it("reads the shed reason off the header, then the body, and frames a bare start 503 as too many streams", () => {
    const { spies, hls } = setup();
    act(() => hls().emit("ERROR", net503("fragLoadError", { networkDetails: { getResponseHeader: () => "file_not_ready" } })));
    expect(spies.setShedReason).toHaveBeenLastCalledWith("file_not_ready");
    act(() => hls().emit("ERROR", net503("manifestLoadError", { networkDetails: { responseText: '{"code":"too_many_streams"}' } })));
    expect(spies.setShedReason).toHaveBeenLastCalledWith("too_many_streams");
    spies.setShedReason.mockClear();
    act(() => hls().emit("ERROR", net503("levelLoadError", { networkDetails: { responseText: "<html>503</html>" } })));
    expect(spies.setShedReason).toHaveBeenLastCalledWith("too_many_streams");
    expect(hls().startLoad).not.toHaveBeenCalled();
    expect(spies.setFatalError).not.toHaveBeenCalled();
  });

  it("retries a bare fragment 503 with backoff and clears the shed on the next fragment", () => {
    vi.useFakeTimers();
    const { spies, hls } = setup();
    act(() => hls().emit("ERROR", net503("fragLoadError")));
    expect(spies.setShedReason).not.toHaveBeenCalled();
    expect(hls().startLoad).not.toHaveBeenCalled();
    act(() => { vi.advanceTimersByTime(2000); });
    expect(hls().startLoad).toHaveBeenCalledTimes(1);
    act(() => hls().emit("FRAG_LOADED", { frag: { type: "main", level: 1, duration: 4, stats: { total: 100, loading: { first: 1, end: 5 } } } }));
    expect(spies.setShedReason).toHaveBeenCalledWith(null);
  });

  it("a 401 refreshes the token, reloads a manifest-level failure with a fresh URL, and only restarts a fragment", async () => {
    const { refs, video, hls } = setup();
    (video as any).currentTime = 42;
    await act(async () => hls().emit("ERROR", { fatal: true, type: "networkError", details: "manifestLoadError", response: { code: 401 } }));
    // The beacon names the recovery, never a dead playback: the token is
    // refreshed right below and playback goes on.
    expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ type: "hls_token_refresh", fatal: false, code: 401 }));
    expect(refs.onStreamTokenRefreshRef.current).toHaveBeenCalledTimes(1);
    expect(refs.streamTokenRef.current).toBe("tok2");
    expect(utils.buildUrl).toHaveBeenLastCalledWith("m1", "tok2", 0, null, 42, 0, false, false, false);
    expect(hls().loadSource).toHaveBeenCalledTimes(2);
    expect(hls().startLoad).toHaveBeenCalledTimes(1);

    await act(async () => hls().emit("ERROR", { fatal: true, type: "networkError", details: "fragLoadError", response: { code: 401 } }));
    expect(hls().loadSource).toHaveBeenCalledTimes(2);
    expect(hls().startLoad).toHaveBeenCalledTimes(2);
  });

  it("a failed token refresh is fatal only for the live instance", async () => {
    const { refs, spies, hls } = setup();
    refs.onStreamTokenRefreshRef.current = vi.fn(async () => { throw new Error("expired"); });
    await act(async () => hls().emit("ERROR", { fatal: true, type: "networkError", details: "fragLoadError", response: { code: 401 } }));
    expect(spies.setFatalError).toHaveBeenCalledWith(true);
  });

  it("a first 404 mid-stream rebuilds at the playhead, a second one kills playback", () => {
    const { video, spies, hook, hls, refs } = setup();
    (video as any).currentTime = 30;
    const first = hls();
    act(() => first.emit("ERROR", { fatal: true, type: "networkError", details: "fragLoadError", response: { code: 404 } }));
    expect(hook.result.current.transcodeStartTime).toBe(30);
    expect(H.FakeHls.instances).toHaveLength(2);
    expect(first.destroy).toHaveBeenCalledTimes(1);
    expect(spies.setFatalError).not.toHaveBeenCalled();

    const second = hls();
    act(() => second.emit("ERROR", { fatal: true, type: "networkError", details: "fragLoadError", response: { code: 410 } }));
    act(() => second.emit("ERROR", { fatal: true, type: "networkError", details: "fragLoadError", response: { code: 404 } }));
    expect(second.destroy).toHaveBeenCalledTimes(1);
    expect(refs.hlsRef.current).toBeNull();
    expect(video.load).toHaveBeenCalledTimes(1);
    expect(spies.setFatalError).toHaveBeenCalledWith(true);
  });

  it("a 404 at position 0 refetches the master in place instead of rebuilding", () => {
    const { hls } = setup();
    act(() => hls().emit("ERROR", { fatal: true, type: "networkError", details: "levelLoadError", response: { code: 404 } }));
    expect(H.FakeHls.instances).toHaveLength(1);
    expect(hls().loadSource).toHaveBeenCalledTimes(2);
    expect(hls().startLoad).toHaveBeenCalledTimes(1);
  });

  it("beacons media errors, recovers a fatal one, and reports a stall", () => {
    const { refs, spies, hls } = setup();
    act(() => hls().emit("ERROR", { fatal: false, type: "mediaError", details: "bufferStalledError" }));
    expect(refs.onBufferStallRef.current).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ type: "hls_media_error", fatal: false }));
    act(() => hls().emit("ERROR", { fatal: true, type: "mediaError", details: "bufferAppendError", frag: { sn: 7, level: 1 } }));
    expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ type: "hls_fatal", segment: 7, variant: 1, state: "snap" }));
    expect(hls().recoverMediaError).toHaveBeenCalledTimes(1);
    act(() => hls().emit("ERROR", { fatal: true, type: "otherError", details: "internalException" }));
    expect(spies.setFatalError).toHaveBeenCalledWith(true);
  });

  it("handoffToTranscode pins vd=transcode on every later rebuild, once", () => {
    const { hook } = setup();
    act(() => hook.result.current.handoffToTranscode());
    expect(H.FakeHls.instances).toHaveLength(2);
    expect(utils.buildUrl.mock.calls.at(-1)?.[8]).toBe(true);
    act(() => hook.result.current.handoffToTranscode());
    expect(H.FakeHls.instances).toHaveLength(2);
  });
});

describe("useTranscodePlayback - native HLS", () => {
  // Safari decodes HLS itself: the element gets the URL, the resume target
  // is applied once metadata is known, and no hls.js instance is built.
  it("sets the element source and seeks on loadedmetadata", () => {
    utils.native = true;
    const { video } = setup({ positionRef: { current: 600 } });
    expect(H.FakeHls.instances).toHaveLength(0);
    expect(video.src).toContain("hls://");
    expect(utils.buildUrl.mock.calls.at(-1)?.[4]).toBe(600);
    act(() => { video.dispatchEvent(new Event("loadedmetadata")); });
    expect(video.currentTime).toBe(600);
    expect(utils.startPlayback).toHaveBeenCalled();
  });
});
