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

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
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
