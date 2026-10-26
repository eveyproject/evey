import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { IMAGE_RETRY_DELAYS_MS, resetImageRetryForTests, useImageRetry } from "./useImageRetry";

const URI = "https://srv/api/v1/media/m1/artwork/poster?v=1&w=288";

describe("useImageRetry", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetImageRetryForTests();
  });
  afterEach(() => vi.useRealTimers());

  it("retries a failed load after a pause on a URL the cache has not seen, then gives up", () => {
    const { result } = renderHook(() => useImageRetry(URI));
    expect(result.current.src).toBe(URI);
    act(() => result.current.onError());
    expect(result.current.src).toBe(URI);
    act(() => vi.advanceTimersByTime(IMAGE_RETRY_DELAYS_MS[0]));
    expect(result.current.src).toBe(`${URI}&r=1`);
    act(() => result.current.onError());
    act(() => vi.advanceTimersByTime(IMAGE_RETRY_DELAYS_MS[1]));
    expect(result.current.src).toBe(`${URI}&r=2`);
    act(() => result.current.onError());
    act(() => vi.advanceTimersByTime(IMAGE_RETRY_DELAYS_MS[2]));
    expect(result.current.src).toBe(`${URI}&r=3`);
    act(() => result.current.onError());
    expect(result.current.failed).toBe(true);
    expect(result.current.src).toBeUndefined();
  });

  it("remembers the attempt that worked for the session, so the next mount starts from it", () => {
    const a = renderHook(() => useImageRetry(URI));
    act(() => a.result.current.onError());
    act(() => vi.advanceTimersByTime(IMAGE_RETRY_DELAYS_MS[0]));
    act(() => a.result.current.onLoad());
    const b = renderHook(() => useImageRetry(URI));
    expect(b.result.current.src).toBe(`${URI}&r=1`);
  });

  it("a new token gives a failed image another go", () => {
    const { result, rerender } = renderHook(({ uri, token }: { uri: string; token: string }) => useImageRetry(uri, token), {
      initialProps: { uri: "https://srv/p", token: "t1" },
    });
    for (const d of IMAGE_RETRY_DELAYS_MS) {
      act(() => result.current.onError());
      act(() => vi.advanceTimersByTime(d));
    }
    act(() => result.current.onError());
    expect(result.current.failed).toBe(true);
    rerender({ uri: "https://srv/p", token: "t2" });
    expect(result.current.failed).toBe(false);
    expect(result.current.src).toBe("https://srv/p");
  });
});
