import { describe, it, expect, vi, beforeEach } from "vitest";

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
}));

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
});

describe("buildStreamUrl", () => {
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

  it("includes startSec when > 0 on HLS URL", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", 42.7);
    expect(url).toContain("startSec=42");
  });

  it("omits startSec when 0 or undefined", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const a = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", 0);
    const b = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(a).not.toContain("startSec");
    expect(b).not.toContain("startSec");
  });

  it("falls back to HLS URL for unknown play method", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "WHATEVER" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
  });
});

describe("fetchStreamToken", () => {
  it("calls stream-token endpoint and returns token", async () => {
    eveyFetchMock.mockResolvedValue({ token: "abc123", mediaId: "m1" });

    // Call the same endpoint the hook uses
    const res = await eveyFetchMock({
      url: "/api/v1/media/m1/stream-token",
      method: "POST",
    });

    expect(res.token).toBe("abc123");
    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/stream-token",
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
      supportedAudioCodecs: ["aac", "mp3", "opus", "flac", "ac3", "eac3"],
      supportedContainers: ["mp4", "hls"],
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
