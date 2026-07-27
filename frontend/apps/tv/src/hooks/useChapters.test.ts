import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

import { useChapters, findCurrentChapterIndex } from "./useChapters";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useChapters", () => {
  it("fetches chapters for a media item", async () => {
    const chapters = [
      { title: "Intro", startTimeMs: 0, endTimeMs: 60000 },
      { title: "Main", startTimeMs: 60000, endTimeMs: 300000 },
    ];
    eveyFetchMock.mockResolvedValue({ chapters });

    const { result } = renderHook(() => useChapters("m1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/media/m1/chapters", method: "GET" });
    expect(result.current.data).toEqual(chapters);
  });

  it("returns empty array when no chapters", async () => {
    eveyFetchMock.mockResolvedValue({ chapters: null });
    const { result } = renderHook(() => useChapters("m2"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns empty array on error", async () => {
    eveyFetchMock.mockRejectedValue(new Error("network"));
    const { result } = renderHook(() => useChapters("m3"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("does not fetch when mediaId is undefined", () => {
    renderHook(() => useChapters(undefined), { wrapper: createWrapper() });
    expect(eveyFetchMock).not.toHaveBeenCalled();
  });
});

describe("findCurrentChapterIndex", () => {
  const chapters = [
    { title: "Intro", startTimeMs: 0, endTimeMs: 60000 },
    { title: "Main", startTimeMs: 60000, endTimeMs: 300000 },
    { title: "Credits", startTimeMs: 300000, endTimeMs: 320000 },
  ];

  it("returns -1 for an empty list", () => {
    expect(findCurrentChapterIndex([], 1000)).toBe(-1);
  });

  it("finds the first chapter at time 0", () => {
    expect(findCurrentChapterIndex(chapters, 0)).toBe(0);
  });

  it("finds the chapter strictly inside its window", () => {
    expect(findCurrentChapterIndex(chapters, 90000)).toBe(1);
  });

  it("treats the last chapter's window as open-ended", () => {
    expect(findCurrentChapterIndex(chapters, 10_000_000)).toBe(2);
  });

  it("picks the exact boundary as the START of the next chapter", () => {
    expect(findCurrentChapterIndex(chapters, 60000)).toBe(1);
  });
});
