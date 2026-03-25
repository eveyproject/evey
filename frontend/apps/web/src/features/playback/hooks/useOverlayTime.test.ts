import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { useOverlayTime } from "./useOverlayTime";

function createVideoElement(overrides: Partial<HTMLVideoElement> = {}) {
  const el = document.createElement("video");
  Object.defineProperty(el, "currentTime", { value: overrides.currentTime ?? 0, writable: true });
  Object.defineProperty(el, "duration", { value: overrides.duration ?? 0, writable: true, configurable: true });
  return el;
}

function makeRefs(video: HTMLVideoElement | null, offset = 0, knownDuration = 0) {
  return {
    videoRef: { current: video },
    transcodeOffsetRef: { current: offset },
    knownDurationSecRef: { current: knownDuration },
  };
}

describe("useOverlayTime", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("returns zero initially", () => {
    const video = createVideoElement();
    const { result } = renderHook(() => useOverlayTime(makeRefs(video)));
    expect(result.current.overlayTime).toBe(0);
    expect(result.current.overlayDuration).toBe(0);
  });

  it("updates overlayTime on timeupdate event", () => {
    const video = createVideoElement({ currentTime: 10 });
    const refs = makeRefs(video, 5);
    const { result } = renderHook(() => useOverlayTime(refs));

    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });

    expect(result.current.overlayTime).toBe(15);
  });

  it("uses knownDurationSec when positive", () => {
    const video = createVideoElement({ currentTime: 0 });
    const refs = makeRefs(video, 0, 3600);
    const { result } = renderHook(() => useOverlayTime(refs));

    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });

    expect(result.current.overlayDuration).toBe(3600);
  });

  it("falls back to video.duration + offset when knownDuration is 0", () => {
    const video = createVideoElement({ currentTime: 5, duration: 100 });
    const refs = makeRefs(video, 20, 0);
    const { result } = renderHook(() => useOverlayTime(refs));

    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });

    expect(result.current.overlayDuration).toBe(120);
  });

  it("does not set duration when video.duration is not finite", () => {
    const video = createVideoElement({ currentTime: 5, duration: Infinity });
    const refs = makeRefs(video, 0, 0);
    const { result } = renderHook(() => useOverlayTime(refs));

    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });

    expect(result.current.overlayDuration).toBe(0);
  });

  it("cleans up event listener on unmount", () => {
    const video = createVideoElement();
    const removeSpy = vi.spyOn(video, "removeEventListener");
    const { unmount } = renderHook(() => useOverlayTime(makeRefs(video)));

    unmount();

    expect(removeSpy).toHaveBeenCalledWith("timeupdate", expect.any(Function));
  });

  it("handles null videoRef gracefully", () => {
    const refs = makeRefs(null);
    const { result } = renderHook(() => useOverlayTime(refs));
    expect(result.current.overlayTime).toBe(0);
    expect(result.current.overlayDuration).toBe(0);
  });
});
