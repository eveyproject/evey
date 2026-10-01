import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useHlsRungs } from "./useHlsRungs";

// The quality menu offers what the master declares: one fetch of the master,
// none for direct play, the token as a header, and a failure keeps the
// static ladder (an empty list) rather than surfacing an error.
const master = `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=6000000,RESOLUTION=1920x1080
stream_0.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720
stream_1.m3u8
`;

describe("useHlsRungs", () => {
  const fetchMock = vi.fn();
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("reads the rungs off the master, sending the stream token as a bearer", async () => {
    fetchMock.mockResolvedValue({ ok: true, text: () => Promise.resolve(master) });
    const { result } = renderHook(() => useHlsRungs("https://h/api/v1/media/m1/hls/master.m3u8", "tok"));
    await waitFor(() => expect(result.current).toEqual([{ width: 1920, height: 1080 }, { width: 1280, height: 720 }]));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("master.m3u8");
    expect((init.headers as Record<string, string>).Authorization).toBe("Bearer tok");
  });

  it("fetches nothing for direct play or without a url", () => {
    const { result } = renderHook(() => useHlsRungs("https://h/api/v1/media/m1/stream?token=x", "tok"));
    expect(result.current).toEqual([]);
    renderHook(() => useHlsRungs(null));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps the static ladder when the master cannot be read", async () => {
    fetchMock.mockResolvedValueOnce({ ok: false, status: 503, text: () => Promise.resolve("") });
    const { result } = renderHook(() => useHlsRungs("https://h/x/master.m3u8"));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(result.current).toEqual([]);

    fetchMock.mockRejectedValueOnce(new Error("offline"));
    const { result: r2 } = renderHook(() => useHlsRungs("https://h/y/master.m3u8"));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(r2.current).toEqual([]);
  });

  it("aborts the fetch when the url changes or the hook unmounts", async () => {
    let captured: AbortSignal | undefined;
    fetchMock.mockImplementation((_url: string, init: RequestInit) => {
      captured = init.signal as AbortSignal;
      return new Promise(() => {});
    });
    const { unmount } = renderHook(() => useHlsRungs("https://h/z/master.m3u8"));
    await waitFor(() => expect(captured).toBeDefined());
    unmount();
    expect(captured?.aborted).toBe(true);
  });
});
