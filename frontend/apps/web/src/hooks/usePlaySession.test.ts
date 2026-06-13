import React from "react";
import { renderHook, cleanup, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const startStreamFn = vi.fn(() => Promise.resolve({ id: "session-123" }));
const fetchStreamTokenFn = vi.fn(() => Promise.resolve("stream-tok-abc"));

vi.mock("@evey/api", () => ({
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  usePlaybackServiceGetStreamInfo: () => ({
    data: { directPlayUrl: "/original-url", videoCodec: "h264" },
  }),
  playbackServiceStartStream: startStreamFn,
}));

vi.mock("./useStreamToken", () => ({
  useStreamToken: () => ({
    streamToken: "stream-tok-abc",
    fetchStreamToken: fetchStreamTokenFn,
    isLoading: false,
  }),
}));

describe("usePlaySession", () => {
  beforeEach(() => {
    startStreamFn.mockClear();
    startStreamFn.mockReturnValue(Promise.resolve({ id: "session-123" }));
    fetchStreamTokenFn.mockClear();
    fetchStreamTokenFn.mockReturnValue(Promise.resolve("stream-tok-abc"));
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ ok: true } as Response)));
    vi.resetModules();
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  function stopCalls() {
    return (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.filter(
      (c: unknown[]) => c[0] === "/api/v1/playback/stop",
    );
  }

  async function loadHook(canPlay: string) {
    vi.spyOn(HTMLVideoElement.prototype, "canPlayType").mockReturnValue(canPlay as CanPlayTypeResult);
    const mod = await import("./usePlaySession");
    return mod.usePlaySession;
  }

  it("initializes session on mount after fetching stream token", async () => {
    const usePlaySession = await loadHook("");
    await act(async () => {
      renderHook(() => usePlaySession("media-1"));
    });
    expect(fetchStreamTokenFn).toHaveBeenCalledWith("media-1");
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

  it("stops session on unmount via a keepalive POST /playback/stop", async () => {
    const usePlaySession = await loadHook("");
    let unmountFn: () => void;
    await act(async () => {
      const { unmount } = renderHook(() => usePlaySession("media-2"));
      unmountFn = unmount;
    });
    act(() => {
      unmountFn();
    });
    const calls = stopCalls();
    expect(calls).toHaveLength(1);
    expect(calls[0][1]).toMatchObject({
      method: "POST",
      keepalive: true,
      body: JSON.stringify({ sessionId: "session-123" }),
    });
  });

  it("does NOT stop on unmount when handing off to the mini-player, but the preserved stop fn still tears the session down", async () => {
    // Regression: clicking the browser back button parks the player as a
    // mini-player (mode = "mini"); the session must survive the route unmount
    // so the mini-player's close button can stop it — otherwise the transcode
    // leaked in the admin panel.
    const usePlaySession = await loadHook("");
    const { usePlayerStore } = await import("@evey/store");
    let result: { current: ReturnType<typeof usePlaySession> };
    let unmountFn: () => void;
    await act(async () => {
      const hook = renderHook(() => usePlaySession("media-mini"));
      result = hook.result;
      unmountFn = hook.unmount;
    });
    // Simulate the player handing off to the mini-player before unmount.
    act(() => {
      usePlayerStore.getState().setMode("mini");
    });
    const stopSession = result!.current.stopSession;
    act(() => {
      unmountFn();
    });
    // Unmount must NOT stop the session while mini-player is active.
    expect(stopCalls()).toHaveLength(0);
    // The preserved stop fn (called later by MiniPlayer's close button) works
    // even though the player route has unmounted.
    act(() => {
      stopSession();
    });
    const calls = stopCalls();
    expect(calls).toHaveLength(1);
    expect(calls[0][1]).toMatchObject({
      body: JSON.stringify({ sessionId: "session-123" }),
    });
    act(() => {
      usePlayerStore.getState().setMode("hidden");
    });
  });

  it("includes stream token in directPlayUrl", async () => {
    const usePlaySession = await loadHook("probably");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      const hook = renderHook(() => usePlaySession("media-hevc"));
      result = hook.result;
    });
    expect(result!.current.streamInfo?.directPlayUrl).toContain("token=stream-tok-abc");
  });

  it("exposes streamToken and fetchStreamToken", async () => {
    const usePlaySession = await loadHook("");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      const hook = renderHook(() => usePlaySession("media-3"));
      result = hook.result;
    });
    expect(result!.current.streamToken).toBe("stream-tok-abc");
    expect(typeof result!.current.fetchStreamToken).toBe("function");
  });
});
