import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useDirectPlayback } from "./useDirectPlayback";
import type { V1StreamInfo } from "@evey/api";

// A <video> the hook can drive: play() resolves, `error` is settable, and
// audioTracks can be faked for the silent-file check.
function makeVideo() {
  const v = document.createElement("video");
  Object.defineProperty(v, "play", { configurable: true, writable: true, value: vi.fn().mockResolvedValue(undefined) });
  let err: MediaError | null = null;
  Object.defineProperty(v, "error", { configurable: true, get: () => err, set: (e: MediaError | null) => (err = e) });
  return v;
}

function mount(video: HTMLVideoElement, overrides: Partial<Parameters<typeof useDirectPlayback>[0]> = {}) {
  const setTranscodeMode = vi.fn();
  const setIsBuffering = vi.fn();
  const onEnded = vi.fn();
  const positionRef = { current: 0 };
  const streamTokenRef = { current: "old" };
  const onStreamTokenRefreshRef = { current: vi.fn().mockResolvedValue("fresh") };
  const args = {
    videoRef: { current: video },
    positionRef,
    streamInfo: { directPlayUrl: "/api/v1/media/m1/stream?token=old" } as V1StreamInfo,
    startPositionMs: 90_000,
    transcodeMode: false,
    onEnded,
    setTranscodeMode,
    setIsBuffering,
    streamTokenRef,
    onStreamTokenRefreshRef,
    ...overrides,
  };
  const hook = renderHook(() => useDirectPlayback(args));
  return { hook, setTranscodeMode, setIsBuffering, onEnded, positionRef, streamTokenRef, onStreamTokenRefreshRef };
}

async function flush() {
  await new Promise((r) => setTimeout(r, 0));
  await new Promise((r) => setTimeout(r, 0));
}

beforeEach(() => {
  vi.spyOn(console, "warn").mockImplementation(() => {});
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("useDirectPlayback", () => {
  it("points the element at the file, seeks to the start position and plays", () => {
    const video = makeVideo();
    const { setTranscodeMode } = mount(video);
    expect(video.src).toContain("/api/v1/media/m1/stream?token=old");
    expect(video.currentTime).toBe(90);
    expect(video.play).toHaveBeenCalled();
    expect(setTranscodeMode).not.toHaveBeenCalled();
  });

  it("falls back to transcode when the server said direct play but gave no URL", () => {
    const video = makeVideo();
    const { setTranscodeMode } = mount(video, { streamInfo: {} as V1StreamInfo });
    expect(setTranscodeMode).toHaveBeenCalledWith(true);
    expect(video.play).not.toHaveBeenCalled();
  });

  it("does nothing while in transcode mode", () => {
    const video = makeVideo();
    mount(video, { transcodeMode: true });
    expect(video.src).toBe("");
  });

  // The most common error after a long pause is the stream token expiring: the
  // element only says "error", so the hook refreshes the token once, keeps the
  // position, and only switches to a transcode on the second failure.
  it("refreshes the token once on error and keeps the position, then falls back", async () => {
    const video = makeVideo();
    const { setTranscodeMode, positionRef, streamTokenRef, onStreamTokenRefreshRef } = mount(video);
    positionRef.current = 1234;
    (video as { error: MediaError | null }).error = { code: 2, message: "network" } as MediaError;

    video.dispatchEvent(new Event("error"));
    await flush();
    expect(onStreamTokenRefreshRef.current).toHaveBeenCalledTimes(1);
    expect(streamTokenRef.current).toBe("fresh");
    expect(video.src).toContain("token=fresh");
    expect(video.src).not.toContain("token=old");
    expect(video.currentTime).toBe(1234);
    expect(video.play).toHaveBeenCalledTimes(2);
    expect(setTranscodeMode).not.toHaveBeenCalled();

    video.dispatchEvent(new Event("error"));
    await flush();
    expect(onStreamTokenRefreshRef.current).toHaveBeenCalledTimes(1);
    expect(setTranscodeMode).toHaveBeenCalledWith(true);
  });

  it("falls back straight away when the URL carries no token or the refresh fails", async () => {
    const plain = makeVideo();
    const first = mount(plain, { streamInfo: { directPlayUrl: "/api/v1/media/m1/stream" } as V1StreamInfo });
    (plain as { error: MediaError | null }).error = { code: 4, message: "unsupported" } as MediaError;
    plain.dispatchEvent(new Event("error"));
    await flush();
    expect(first.onStreamTokenRefreshRef.current).not.toHaveBeenCalled();
    expect(first.setTranscodeMode).toHaveBeenCalledWith(true);

    const video = makeVideo();
    const second = mount(video, { onStreamTokenRefreshRef: { current: vi.fn().mockRejectedValue(new Error("401")) } });
    (video as { error: MediaError | null }).error = { code: 2, message: "network" } as MediaError;
    video.dispatchEvent(new Event("error"));
    await flush();
    expect(second.setTranscodeMode).toHaveBeenCalledWith(true);

    // An error event with no MediaError behind it is ignored.
    const quiet = makeVideo();
    const third = mount(quiet);
    quiet.dispatchEvent(new Event("error"));
    await flush();
    expect(third.setTranscodeMode).not.toHaveBeenCalled();
  });

  it("reports buffering only while playing, and the end of the file", () => {
    const video = makeVideo();
    const { setIsBuffering, onEnded, positionRef } = mount(video);
    Object.defineProperty(video, "paused", { configurable: true, get: () => true });
    video.dispatchEvent(new Event("waiting"));
    expect(setIsBuffering).not.toHaveBeenCalledWith(true);
    Object.defineProperty(video, "paused", { configurable: true, get: () => false });
    video.dispatchEvent(new Event("waiting"));
    expect(setIsBuffering).toHaveBeenCalledWith(true);
    video.dispatchEvent(new Event("playing"));
    expect(setIsBuffering).toHaveBeenLastCalledWith(false);
    video.currentTime = 42;
    video.dispatchEvent(new Event("timeupdate"));
    expect(positionRef.current).toBe(42);
    video.dispatchEvent(new Event("ended"));
    expect(onEnded).toHaveBeenCalled();
  });

  // A file whose audio the browser cannot decode plays SILENT: the element
  // reports zero audio tracks a moment after canplay, and that is a transcode.
  it("switches to transcode when the element exposes no audio track after canplay", () => {
    vi.useFakeTimers();
    const video = makeVideo();
    const { setTranscodeMode, hook } = mount(video);
    Object.defineProperty(video, "audioTracks", { configurable: true, get: () => ({ length: 0 }) });
    video.dispatchEvent(new Event("canplay"));
    expect(setTranscodeMode).not.toHaveBeenCalled();
    vi.advanceTimersByTime(3000);
    expect(setTranscodeMode).toHaveBeenCalledWith(true);

    // Unmounting cancels a pending check.
    const other = makeVideo();
    const second = mount(other);
    Object.defineProperty(other, "audioTracks", { configurable: true, get: () => ({ length: 0 }) });
    other.dispatchEvent(new Event("canplay"));
    second.hook.unmount();
    vi.advanceTimersByTime(3000);
    expect(second.setTranscodeMode).not.toHaveBeenCalled();
    hook.unmount();
  });
});

// The refresh re-renders the parent with the fresh-token URL, which re-runs
// the effect. A file the browser cannot open at all (Firefox on a .mkv) then
// errors again: the retry must not be re-armed by the re-run, or the player
// reloads the source forever (660 reloads a minute, measured 2026-09-29).
it("does not retry again after the parent re-renders with the refreshed URL", async () => {
  const video = makeVideo();
  const setTranscodeMode = vi.fn();
  const onStreamTokenRefreshRef = { current: vi.fn().mockResolvedValue("fresh") };
  const base = {
    videoRef: { current: video },
    positionRef: { current: 0 },
    streamInfo: { directPlayUrl: "/api/v1/media/m1/stream?token=old" } as V1StreamInfo,
    startPositionMs: 0,
    transcodeMode: false,
    onEnded: vi.fn(),
    setTranscodeMode,
    setIsBuffering: vi.fn(),
    streamTokenRef: { current: "old" },
    onStreamTokenRefreshRef,
  };
  const hook = renderHook((p: typeof base) => useDirectPlayback(p), { initialProps: base });
  (video as { error: MediaError | null }).error = { code: 4, message: "Failed to init decoder" } as MediaError;
  video.dispatchEvent(new Event("error"));
  await flush();
  expect(onStreamTokenRefreshRef.current).toHaveBeenCalledTimes(1);
  expect(setTranscodeMode).not.toHaveBeenCalled();

  hook.rerender({ ...base, streamInfo: { directPlayUrl: "/api/v1/media/m1/stream?token=fresh" } as V1StreamInfo });
  video.dispatchEvent(new Event("error"));
  await flush();
  expect(onStreamTokenRefreshRef.current).toHaveBeenCalledTimes(1);
  expect(setTranscodeMode).toHaveBeenCalledWith(true);
});
