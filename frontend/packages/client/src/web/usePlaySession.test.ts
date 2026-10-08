import React from "react";
import { renderHook, cleanup, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const startStreamFn = vi.fn(() => Promise.resolve({ id: "session-123" }));
const fetchStreamTokenFn = vi.fn(() => Promise.resolve("stream-tok-abc"));
const getStreamInfoFn = vi.fn((_mediaId: string, _params: unknown) => ({
  data: { directPlayUrl: "/original-url", videoCodec: "h264" },
}));

vi.mock("@evey/api", () => ({
  // The web app: same origin, so the stop beacon goes to a relative path.
  getBaseURL: () => "",
  getCredentialsMode: () => "include",
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  usePlaybackServiceGetStreamInfo: (...a: [string, unknown]) => getStreamInfoFn(...a),
  playbackServiceStartStream: startStreamFn,
}));
// The browser's output ceiling (stereo speakers here) - the same reading
// appendCodecParams puts on the master URL.
vi.mock("./outputChannels", () => ({ getMaxOutputChannels: () => 2 }));

vi.mock("../hooks/useStreamToken", () => ({
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
    expect(fetchStreamTokenFn).toHaveBeenCalledWith("media-1", undefined);
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

  it("stops the session on unmount even after the player mode changed (no mini-player handoff)", async () => {
    // Leaving the player always ends the session now - there is no mini-player
    // that keeps the stream (and its admin "Now Playing" entry) alive in the
    // background. The browser back button unmounts the route → session stops.
    const usePlaySession = await loadHook("");
    const { usePlayerStore } = await import("@evey/store");
    let unmountFn: () => void;
    await act(async () => {
      const hook = renderHook(() => usePlaySession("media-mini"));
      unmountFn = hook.unmount;
    });
    act(() => {
      usePlayerStore.getState().setMode("full");
    });
    act(() => {
      unmountFn();
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

  // The verdict is asked with the output's channel ceiling: declared to the
  // master URL and not to GetStreamInfo, the panel said "copy 6ch" of a track
  // the segments carried folded to stereo.
  it("asks the stream-info verdict with the output's channel ceiling", async () => {
    const usePlaySession = await loadHook("");
    await act(async () => {
      renderHook(() => usePlaySession("media-4"));
    });
    expect(getStreamInfoFn).toHaveBeenCalledWith("media-4", expect.objectContaining({ maxAudioChannels: 2 }));
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
