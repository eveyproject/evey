import React from "react";
import { renderHook, cleanup, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const startStreamFn = vi.fn(() => Promise.resolve({ id: "session-123" }));
const stopMutateFn = vi.fn();

vi.mock("@evey/api", () => ({
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  usePlaybackServiceGetStreamInfo: () => ({
    data: { directPlayUrl: "/original-url", videoCodec: "h264" },
  }),
  playbackServiceStartStream: startStreamFn,
  usePlaybackServiceStopStream: () => ({ mutate: stopMutateFn }),
}));

vi.mock("@evey/api/src/generated/playback.schemas", () => ({}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    () => null,
    { getState: () => ({ accessToken: "test-token" }) },
  ),
}));

describe("usePlaySession", () => {
  beforeEach(() => {
    startStreamFn.mockClear();
    startStreamFn.mockReturnValue(Promise.resolve({ id: "session-123" }));
    stopMutateFn.mockClear();
    vi.resetModules();
  });

  afterEach(() => {
    cleanup();
  });

  async function loadHook(canPlay: string) {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue(canPlay as CanPlayTypeResult);
    const mod = await import("./usePlaySession");
    return mod.usePlaySession;
  }

  it("initializes session on mount", async () => {
    const usePlaySession = await loadHook("");
    await act(async () => {
      renderHook(() => usePlaySession("media-1"));
    });
    expect(startStreamFn).toHaveBeenCalledTimes(1);
    expect(startStreamFn).toHaveBeenCalledWith(
      expect.objectContaining({ mediaItemId: "media-1" }),
    );
  });

  it("prevents double start (StrictMode guard)", async () => {
    const usePlaySession = await loadHook("");
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(React.StrictMode, null, children);

    await act(async () => {
      renderHook(() => usePlaySession("media-strict"), { wrapper });
    });
    expect(startStreamFn).toHaveBeenCalledTimes(1);
  });

  it("stops session on unmount", async () => {
    const usePlaySession = await loadHook("");
    let unmountFn: () => void;
    await act(async () => {
      const { unmount } = renderHook(() => usePlaySession("media-2"));
      unmountFn = unmount;
    });
    act(() => {
      unmountFn();
    });
    expect(stopMutateFn).toHaveBeenCalledWith(
      expect.objectContaining({
        data: { sessionId: "session-123" },
      }),
    );
  });

  it("includes token in directPlayUrl when session starts", async () => {
    const usePlaySession = await loadHook("probably");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      const hook = renderHook(() => usePlaySession("media-hevc"));
      result = hook.result;
    });
    expect(result!.current.streamInfo?.directPlayUrl).toContain("token=test-token");
  });
});
