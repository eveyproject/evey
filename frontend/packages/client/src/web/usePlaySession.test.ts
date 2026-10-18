import React from "react";
import { renderHook, cleanup, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const startStreamFn = vi.fn(() => Promise.resolve({ id: "session-123" }));
const fetchStreamTokenFn = vi.fn(() => Promise.resolve("stream-tok-abc"));
const getStreamInfoFn = vi.fn((_mediaId: string, _params: unknown) => ({
  data: { directPlayUrl: "/original-url", videoCodec: "h264" },
}));
// The verdict asked again for another track (the query above is the first one).
const askStreamInfoFn = vi.fn((_mediaId: string, _params: unknown): Promise<unknown> => Promise.resolve({}));

vi.mock("@evey/api", () => ({
  // The web app: same origin, so the stop beacon goes to a relative path.
  getBaseURL: () => "",
  getCredentialsMode: () => "include",
  usePlaybackServiceGetWatchProgress: () => ({ data: undefined }),
  usePlaybackServiceGetStreamInfo: (...a: [string, unknown]) => getStreamInfoFn(...a),
  playbackServiceGetStreamInfo: (...a: [string, unknown]) => askStreamInfoFn(...a),
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
    getStreamInfoFn.mockReset();
    getStreamInfoFn.mockImplementation(() => ({ data: { directPlayUrl: "/original-url", videoCodec: "h264" } }));
    askStreamInfoFn.mockReset();
    askStreamInfoFn.mockImplementation(() => Promise.resolve({}));
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

  // Two audio tracks, the AAC flagged default: the first verdict, asked
  // without a track, is the AAC's.
  const TWO_TRACKS = {
    playMethod: "PLAY_METHOD_DIRECT_PLAY",
    audioDecision: "copy",
    mediaStreams: [
      { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
      { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", channels: 2, isDefault: true },
      { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "dts", channels: 6 },
    ],
  };

  // The track chosen on the title page rides the first negotiation, as on the
  // native players: asked without it, the server judged the default track.
  it("asks the first verdict for the audio track picked before playback", async () => {
    const usePlaySession = await loadHook("");
    await act(async () => {
      renderHook(() => usePlaySession("media-5", undefined, { audioStreamIndex: 2 }));
    });
    expect(getStreamInfoFn).toHaveBeenCalledWith("media-5", expect.objectContaining({ audioStreamIndex: 2 }));
  });

  it("asks the verdict again for the track the viewer switches to, and exposes that one", async () => {
    getStreamInfoFn.mockReturnValue({ data: { ...TWO_TRACKS } as never });
    askStreamInfoFn.mockResolvedValueOnce({ ...TWO_TRACKS, playMethod: "PLAY_METHOD_TRANSCODE", audioDecision: "transcode" });
    const usePlaySession = await loadHook("");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      result = renderHook(() => usePlaySession("media-6")).result;
    });
    await act(async () => {
      result.current.selectAudioStream(2);
    });
    // Same declaration as the first verdict, the track added.
    expect(askStreamInfoFn).toHaveBeenCalledWith("media-6", expect.objectContaining({ audioStreamIndex: 2, maxAudioChannels: 2 }));
    expect(result!.current.streamInfo?.audioDecision).toBe("transcode");
    expect(result!.current.streamInfo?.playMethod).toBe("PLAY_METHOD_TRANSCODE");
    // A new verdict is not a new play session.
    expect(startStreamFn).toHaveBeenCalledTimes(1);
  });

  it("does not ask again for the track the verdict already describes", async () => {
    getStreamInfoFn.mockReturnValue({ data: { ...TWO_TRACKS } as never });
    const usePlaySession = await loadHook("");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      result = renderHook(() => usePlaySession("media-7")).result;
    });
    await act(async () => {
      result.current.selectAudioStream(1);
    });
    expect(askStreamInfoFn).not.toHaveBeenCalled();
  });

  // Two quick picks: the answer for the track the viewer has already left
  // lands last and must not win.
  it("drops the answer for a track the viewer has since left", async () => {
    getStreamInfoFn.mockReturnValue({ data: { ...TWO_TRACKS } as never });
    let answerFirst: (v: unknown) => void = () => {};
    askStreamInfoFn.mockImplementationOnce(() => new Promise((r) => { answerFirst = r; }));
    const usePlaySession = await loadHook("");
    let result: { current: ReturnType<typeof usePlaySession> };
    await act(async () => {
      result = renderHook(() => usePlaySession("media-8")).result;
    });
    await act(async () => {
      result.current.selectAudioStream(2);
    });
    // Back on the default track: the first verdict describes it again.
    await act(async () => {
      result.current.selectAudioStream(1);
    });
    await act(async () => {
      answerFirst({ ...TWO_TRACKS, audioDecision: "transcode" });
    });
    expect(result!.current.streamInfo?.audioDecision).toBe("copy");
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
