import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const eveyFetchMock = vi.fn();
const getStreamInfoMock = vi.fn();
const startStreamMock = vi.fn();
const stopStreamMock = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  eveyFetchRaw: vi.fn(),
  playbackServiceGetStreamInfo: (...args: unknown[]) => getStreamInfoMock(...args),
  playbackServiceStartStream: (...args: unknown[]) => startStreamMock(...args),
  playbackServiceStopStream: (...args: unknown[]) => stopStreamMock(...args),
}));

vi.mock("../lib/server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("http://localhost:8080"),
}));

vi.mock("react-native", () => ({
  Dimensions: {
    get: () => ({ width: 360, height: 800, scale: 2 }),
  },
  Platform: { OS: "android" },
  // deviceCodecs reads NativeModules.MediaCaps at import time; absent = the
  // conservative baseline audio codec list.
  NativeModules: {},
}));

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
});

describe("buildStreamUrl", () => {
  it("percent-encodes a deep-link traversal id so the stream URL cannot pivot to another API path", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "../admin/api-keys", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok");
    expect(url).toBe("http://srv/api/v1/media/..%2Fadmin%2Fapi-keys/stream?token=tok");
  });

  it("returns direct stream URL for PLAY_METHOD_DIRECT_PLAY (no codec params)", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok");
    expect(url).toBe("http://srv/api/v1/media/m1/stream?token=tok");
  });

  it("returns remux URL for PLAY_METHOD_DIRECT_STREAM with codec params", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_STREAM" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/transcode\?/);
    expect(url).toContain("token=tok");
    expect(url).toContain("remux=1");
    expect(url).toContain("videoCodecs=");
    expect(url).toContain("audioCodecs=");
    expect(url).toContain("containers=");
  });

  it("returns HLS URL for PLAY_METHOD_TRANSCODE with codec params", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=");
  });

  it("declares mkv/webm containers on Android so a compatible MKV direct-plays", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    // Without mkv here the server flags container_incompatible and transcodes
    // every .mkv - the root cause of broken seeking + shifted colors.
    expect(decodeURIComponent(url)).toContain("containers=mp4,mkv,webm,hls");
  });

  // startSec was dead on arrival: no HLS handler parses it. The transcode path
  // reads only maxWidth/maxHeight/maxBitrate/videoCodecs/audioCodecs/containers/
  // hdr/fp (transcode_handler.go:298-322) and audioTrack/subtitleTrack. The
  // server always publishes the VOD manifest from t=0, so emitting startSec made
  // the client believe the timeline was shifted: it set transcodeOffset=startSec
  // and added it to every timeUpdate, over a playlist that still began at 0.
  // Result: position displayed and reported at roughly twice the real value.
  it("never emits startSec - the server ignores it and it doubled the reported position", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", 42.7);
    expect(url).not.toContain("startSec");
  });

  it("falls back to HLS URL for unknown play method", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "WHATEVER" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
  });
});

describe("fetchStreamToken", () => {
  it("requests the long-lived native-purpose token and returns it", async () => {
    eveyFetchMock.mockResolvedValue({ token: "abc123", mediaId: "m1" });
    const { fetchStreamToken } = await import("./usePlaySession");

    const token = await fetchStreamToken("m1");

    expect(token).toBe("abc123");
    // Plan B P3: the native player can't re-mint mid-stream, so it must ask for
    // the 12h window (purpose=native), not the 5-minute default.
    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/stream-token?purpose=native",
      method: "POST",
    });
  });
});

describe("usePlaySession integration", () => {
  it("calls getStreamInfo with supported codecs", async () => {
    const { renderHook } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
    });
    eveyFetchMock
      .mockResolvedValueOnce(null) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    // Wait for loading to finish
    const { waitFor } = await import("@testing-library/react");
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({
      supportedCodecs: ["h264", "hevc", "vp9"],
      // Baseline list: no MediaCaps native module in the test env, and never
      // ac3/eac3 without a probed Dolby decoder (silent direct-play otherwise).
      supportedAudioCodecs: ["aac", "mp3", "opus", "flac", "vorbis"],
      supportedContainers: ["mp4", "mkv", "webm", "hls"],
    }));
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
    // Never register a play session for a request the server refused.
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

  it("defaults to too_many_streams when the 503 carries no usable header or body", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: () => null },
      json: async () => { throw new Error("not json"); },
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("too_many_streams");
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
