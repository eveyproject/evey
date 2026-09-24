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

  it("finds previous episode from siblings", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep2, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.previousEpisode?.id).toBe("ep1");
  });

  it("returns null previousEpisode for the first episode", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    expect(result.current.previousEpisode).toBeNull();
  });

  it("playPrevious navigates to the previous episode", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep2, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    act(() => { result.current.playPrevious(); });
    expect(onNavigate).toHaveBeenCalledWith("ep1");
  });

  it("playPrevious is a no-op on the first episode", () => {
    const onNavigate = vi.fn();
    const { result } = renderHook(
      () => useNextEpisode({ media: ep1, currentTime: 0, duration: 100, onNavigate }),
      { wrapper: createWrapper() },
    );

    act(() => { result.current.playPrevious(); });
    expect(onNavigate).not.toHaveBeenCalled();
  });

  // Item 2: the per-profile "auto-play next episode" preference. Off means
  // the card stays put instead of counting down into another episode on its
  // own - only an explicit playNow/playPrevious may navigate.
  describe("autoAdvance: false", () => {
    it("never navigates on its own, even long after the video ended", () => {
      const onNavigate = vi.fn();
      const { result, rerender } = renderHook(
        ({ currentTime }) =>
          useNextEpisode({ media: ep1, currentTime, duration: 100, onNavigate, autoAdvance: false }),
        { initialProps: { currentTime: 50 }, wrapper: createWrapper() },
      );

      rerender({ currentTime: 99.8 });
      act(() => { vi.advanceTimersByTime(6000); });

      expect(onNavigate).not.toHaveBeenCalled();
      expect(result.current.visible).toBe(true);
    });

    it("playNow still navigates", () => {
      const onNavigate = vi.fn();
      const { result } = renderHook(
        () => useNextEpisode({ media: ep1, currentTime: 75, duration: 100, onNavigate, autoAdvance: false }),
        { wrapper: createWrapper() },
      );

      act(() => { result.current.playNow(); });
      expect(onNavigate).toHaveBeenCalledWith("ep2");
    });
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

// What the web's own overlay already guarded, and the shared hook (the phone,
// both televisions) did not.
describe("useNextEpisode - the web's guards", () => {
  it("clears the end state when the viewer seeks back from the end", () => {
    const onNavigate = vi.fn();
    const { result, rerender } = renderHook(
      ({ t }: { t: number }) => useNextEpisode({ media: ep1, currentTime: t, duration: 100, onNavigate, autoAdvance: false }),
      { wrapper: createWrapper(), initialProps: { t: 100 } },
    );
    expect(result.current.ended).toBe(true);
    rerender({ t: 20 });
    expect(result.current.ended).toBe(false);
    expect(result.current.visible).toBe(false);
  });

  it("moves to the next episode once, even when the caller hands a new callback afterwards", () => {
    const first = vi.fn();
    const { rerender } = renderHook(
      ({ cb }: { cb: (id: string) => void }) => useNextEpisode({ media: ep1, currentTime: 100, duration: 100, onNavigate: cb }),
      { wrapper: createWrapper(), initialProps: { cb: first } },
    );
    act(() => { vi.advanceTimersByTime(6000); });
    expect(first).toHaveBeenCalledTimes(1);
    const second = vi.fn();
    rerender({ cb: second });
    act(() => { vi.advanceTimersByTime(1000); });
    expect(second).not.toHaveBeenCalled();
  });
});
