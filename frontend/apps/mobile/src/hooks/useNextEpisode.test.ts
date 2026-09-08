import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const listMediaItemsMock = vi.fn().mockReturnValue({ data: undefined });
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
}));

import { useNextEpisode } from "./useNextEpisode";
import type { V1MediaItem } from "@evey/api";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function makeEpisode(id: string, index: number, parentId = "season-1"): V1MediaItem {
  return { id, type: "MEDIA_ITEM_TYPE_EPISODE", parentId, indexNumber: index } as V1MediaItem;
}

const ep1 = makeEpisode("ep1", 1);
const ep2 = makeEpisode("ep2", 2);
const ep3 = makeEpisode("ep3", 3);

beforeEach(() => {
  vi.useFakeTimers();
  listMediaItemsMock.mockReset();
  listMediaItemsMock.mockReturnValue({ data: { items: [ep1, ep2, ep3] } });
});
afterEach(() => { vi.useRealTimers(); });

describe("useNextEpisode", () => {
  it("finds next episode from siblings", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.nextEpisode?.id).toBe("ep2");
  });

  it("returns null nextEpisode for last episode", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep3, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.nextEpisode).toBeNull();
  });

  it("visible when in credits zone", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 75, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.visible).toBe(true);
  });

  it("not visible when far from end", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 10, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.visible).toBe(false);
  });

  it("detects video ended state", () => {
    const onNavigate = vi.fn();
    const { result, rerender } = renderHook(
      ({ currentTime }) => useNextEpisode({ media: ep1, currentTime, duration: 100, onNavigate }),
      { initialProps: { currentTime: 50 }, wrapper: createWrapper() },
    );

    expect(result.current.ended).toBe(false);

    rerender({ currentTime: 99.8 });
    expect(result.current.ended).toBe(true);
    expect(result.current.visible).toBe(true);
  });

  it("cancel stops auto-navigation", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 99.8, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    act(() => { result.current.cancel(); });
    act(() => { vi.advanceTimersByTime(10000); });

    expect(onNavigate).not.toHaveBeenCalled();
    expect(result.current.visible).toBe(false);
  });

  it("playNow navigates immediately", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 75, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    act(() => { result.current.playNow(); });
    expect(onNavigate).toHaveBeenCalledWith("ep2");
  });

  it("with autoAdvance false, ends without ever auto-navigating, but playNow still works", () => {
    const onNavigate = vi.fn();
    const { result, rerender } = renderHook(
      ({ currentTime }) => useNextEpisode({ media: ep1, currentTime, duration: 100, onNavigate, autoAdvance: false }),
      { initialProps: { currentTime: 50 }, wrapper: createWrapper() },
    );

    rerender({ currentTime: 99.8 });
    expect(result.current.ended).toBe(true);
    expect(result.current.visible).toBe(true);

    act(() => { vi.advanceTimersByTime(6000); });
    expect(onNavigate).not.toHaveBeenCalled();
    expect(result.current.visible).toBe(true);

    act(() => { result.current.playNow(); });
    expect(onNavigate).toHaveBeenCalledWith("ep2");
  });

  it("not visible for non-episode media", () => {
    listMediaItemsMock.mockReturnValue({ data: undefined });
    const movie = { id: "m1", type: "MEDIA_ITEM_TYPE_MOVIE" } as V1MediaItem;
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: movie, currentTime: 99, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.nextEpisode).toBeNull();
    expect(result.current.visible).toBe(false);
  });
});
