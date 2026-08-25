import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const eveyFetchMock = vi.fn();
const getStreamInfoMock = vi.fn();
const startStreamMock = vi.fn();
const stopStreamMock = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  playbackServiceGetStreamInfo: (...args: unknown[]) => getStreamInfoMock(...args),
  playbackServiceStartStream: (...args: unknown[]) => startStreamMock(...args),
  playbackServiceStopStream: (...args: unknown[]) => stopStreamMock(...args),
}));

vi.mock("../lib/server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("http://localhost:8080"),
}));

const getAudioProbeMock = vi.fn();
vi.mock("@evey/media-caps", () => ({
  getAudioProbe: () => getAudioProbeMock(),
}));

// Minimal shape of the native audio probe; only the two fields the channel
// ceiling is derived from vary between cases.
function audioProbe(over: Record<string, unknown>) {
  return {
    apiLevel: 34,
    isTv: true,
    deviceModel: "test",
    decoders: [],
    directPlayback: {},
    directProfiles: [],
    hdmi: { available: false },
    surroundOutputRaw: -1,
    outputDeviceTypes: [],
    ...over,
  };
}

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
  getAudioProbeMock.mockReset();
  getAudioProbeMock.mockResolvedValue(null);
});

// The channel ceiling of the OUTPUT, which no playback request used to carry:
// the server profile pinned MaxAudioChannels = 6 and the box folded 5.1 onto
// a 2.0 television without normalisation - voices fine, music and effects
// clipping.
describe("maxAudioChannels reaches the stream URL", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
  });

  afterEach(() => {
    global.fetch = realFetch;
  });

  async function negotiatedUrl(): Promise<string> {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.streamUrl).not.toBeNull());
    return result.current.streamUrl as string;
  }

  it("declares the stereo ceiling of a 2.0 television", async () => {
    getAudioProbeMock.mockResolvedValue(
      audioProbe({ hdmi: { available: true, plugState: 1, maxChannelCount: 2 }, outputDeviceTypes: [9] }),
    );
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("2");
  });

  it("leaves a 5.1 amplifier at 5.1", async () => {
    getAudioProbeMock.mockResolvedValue(
      audioProbe({ hdmi: { available: true, plugState: 1, maxChannelCount: 6 }, outputDeviceTypes: [10] }),
    );
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("6");
  });

  it("omits the param when the box cannot answer, keeping the server default", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    expect(new URL(await negotiatedUrl()).searchParams.has("maxAudioChannels")).toBe(false);
  });
});

describe("usePlaySession resume", () => {
  it("fetches watch progress and passes its position to startStream", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "42000", played: false }) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playback/m1/progress",
      method: "GET",
    });
    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "42000" }),
    );
    expect(result.current.progress).toEqual({ positionMs: "42000", played: false });
  });

  it("falls back to position 0 when the item is already fully watched", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "999000", played: true })
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "0" }),
    );
  });

  it("falls back to position 0 when the progress fetch fails", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockRejectedValueOnce(new Error("network")) // progress fetch fails
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "0" }),
    );
    expect(result.current.progress).toBeNull();
  });

  it("sets isError on failure", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockRejectedValue(new Error("network"));
    eveyFetchMock.mockRejectedValue(new Error("network"));

    const { result } = renderHook(() => usePlaySession("m2"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isError).toBe(true);
  });
});

// Server-side load shed (HTTP 503): expo-video's native player only exposes a
// free-text error message, so usePlaySession probes the manifest URL itself
// before handing it to the player - this is the only place that can read the
// server's structured shed code.
describe("buildStreamUrl", () => {
  // startSec was dead on arrival: no HLS handler parses it (transcode_handler.go
  // reads only maxWidth/maxHeight/maxBitrate/videoCodecs/audioCodecs/containers/
  // hdr/fp). The server always publishes the VOD manifest from t=0, so emitting
  // it made the client believe the timeline was shifted: it set
  // transcodeOffset=startSec and added that to every timeUpdate - and on TV to
  // the reported duration as well - over a playlist that still began at 0.
  it("carries the probed channel ceiling, and nothing when there is none", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", 2)).toContain("maxAudioChannels=2");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", 6)).toContain("maxAudioChannels=6");
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("maxAudioChannels");
  });

  it("never emits startSec - the server ignores it and it doubled the reported position", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).not.toContain("startSec");
  });
});

describe("usePlaySession: server load-shed (503)", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
  });

  afterEach(() => {
    global.fetch = realFetch;
  });

  function mockHappyNegotiation() {
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
    eveyFetchMock
      .mockResolvedValueOnce(null) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
  }

  it("reads the shed code off the X-Evey-Error header and does not start a play session", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: (name: string) => (name === "X-Evey-Error" ? "too_many_streams" : null) },
      json: async () => ({ code: "too_many_streams", retryAfterSec: 5, message: "shed: too many streams" }),
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("too_many_streams");
    expect(result.current.isError).toBe(false);
    expect(result.current.streamUrl).toBeNull();
    expect(startStreamMock).not.toHaveBeenCalled();
  });

  it("reads file_not_ready from the JSON body when the header is absent", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: () => null },
      json: async () => ({ code: "file_not_ready", retryAfterSec: 10, message: "shed: index building" }),
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("file_not_ready");
  });

  it("does not probe for PLAY_METHOD_DIRECT_PLAY (nothing to shed)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.shedReason).toBeNull();
    expect(result.current.streamUrl).not.toBeNull();
  });

  it("proceeds normally when the preflight succeeds (2xx)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    startStreamMock.mockResolvedValue({ id: "sess1" });
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBeNull();
    expect(result.current.streamUrl).not.toBeNull();
    expect(startStreamMock).toHaveBeenCalled();
  });
});

// The TV player is a native segment fetcher (expo-video/ExoPlayer): it cannot
// re-mint the 5-minute sliding token mid-stream, so a living-room pause longer
// than the default TTL 401'd the next segment. Mobile got purpose=native long
// ago; the TV had been left behind.
describe("usePlaySession stream token", () => {
  it("requests the long-lived native token, not the 5-minute default", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "0", played: false })
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ url: expect.stringContaining("stream-token?purpose=native") }),
    );
  });

  it("mints a fresh token on restart instead of replaying the dead one", async () => {
    const { renderHook, waitFor, act } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "0", played: false })
      .mockResolvedValueOnce({ token: "t-dead", mediaId: "m1" })
      .mockResolvedValue({ token: "t-fresh", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
    stopStreamMock.mockResolvedValue({});

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.restartWithTracks());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    // Retry exists to recover from a 401: reusing the cached token rebuilds
    // the same dead URL and fails identically, trapping the viewer.
    const tokenCalls = eveyFetchMock.mock.calls.filter(
      (c: unknown[]) => String((c[0] as { url?: string })?.url ?? "").includes("stream-token"),
    );
    expect(tokenCalls.length).toBeGreaterThanOrEqual(2);
  });
});
