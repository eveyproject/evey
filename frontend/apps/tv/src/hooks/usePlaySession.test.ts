import { describe, it, expect, vi, beforeEach } from "vitest";

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
