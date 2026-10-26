import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

const prefetchQuery = vi.fn();
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ prefetchQuery }) }));
vi.mock("@evey/api", () => ({
  getLibraryServiceGetMediaItemQueryOptions: (id: string) => ({ queryKey: [`/api/v1/media/${id}`] }),
}));

import { PREFETCH_DWELL_MS, useMediaPrefetch } from "./useMediaPrefetch";

describe("useMediaPrefetch", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    prefetchQuery.mockClear();
  });
  afterEach(() => vi.useRealTimers());

  it("fetches the page of the card the focus rests on, under the page's own query key", () => {
    const { result } = renderHook(() => useMediaPrefetch());
    act(() => result.current.prefetchOnFocus("m1"));
    act(() => vi.advanceTimersByTime(PREFETCH_DWELL_MS - 1));
    expect(prefetchQuery).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(prefetchQuery).toHaveBeenCalledWith({ queryKey: ["/api/v1/media/m1"] });
  });

  it("a card the focus only crosses costs nothing: the next card or a blur cancels it", () => {
    const a = renderHook(() => useMediaPrefetch());
    const b = renderHook(() => useMediaPrefetch());
    act(() => a.result.current.prefetchOnFocus("m1"));
    act(() => vi.advanceTimersByTime(100));
    // Another card, another component: still one pending prefetch for the app.
    act(() => b.result.current.prefetchOnFocus("m2"));
    act(() => vi.advanceTimersByTime(PREFETCH_DWELL_MS));
    expect(prefetchQuery.mock.calls.map((c) => c[0].queryKey[0])).toEqual(["/api/v1/media/m2"]);
    act(() => a.result.current.prefetchOnFocus("m3"));
    act(() => a.result.current.cancelPrefetch());
    act(() => vi.advanceTimersByTime(PREFETCH_DWELL_MS));
    expect(prefetchQuery).toHaveBeenCalledTimes(1);
  });
});
