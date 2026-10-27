import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";

const listMock = vi.fn();
vi.mock("@evey/api", () => ({ libraryServiceListMediaItems: (...a: unknown[]) => listMock(...a) }));

import { useRowPages } from "./useRowPages";

const items = (from: number, n: number) => Array.from({ length: n }, (_, i) => ({ id: `m${from + i}` }));
const params = { libraryId: "lib", "pagination.pageSize": 8 };

// The first page is react-query's data in the app, the same object across
// renders; a literal inside the render callback would restart the row forever.
describe("useRowPages", () => {
  beforeEach(() => listMock.mockReset());

  it("shows the first page, then fetches the next from its cursor, up to the row's budget", async () => {
    const first = { items: items(0, 8), pagination: { nextCursor: "c1" } };
    listMock.mockResolvedValueOnce({ items: items(8, 8), pagination: { nextCursor: "c2" } });
    const { result } = renderHook(() => useRowPages(params, first, 8, 12));
    expect(result.current.items).toHaveLength(8);
    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.items).toHaveLength(12));
    // Asked for no more than the budget left, from the first page's cursor.
    expect(listMock).toHaveBeenCalledWith({ libraryId: "lib", "pagination.pageSize": 4, "pagination.cursor": "c1" });
    act(() => result.current.loadMore());
    expect(listMock).toHaveBeenCalledTimes(1);
  });

  it("a last page (no cursor) fetches nothing more", () => {
    const last = { items: items(0, 3), pagination: {} };
    const { result } = renderHook(() => useRowPages(params, last, 8, 20));
    act(() => result.current.loadMore());
    expect(listMock).not.toHaveBeenCalled();
    expect(result.current.items).toHaveLength(3);
  });

  it("one fetch at a time, however often the end is reached", async () => {
    let resolve: (v: unknown) => void = () => {};
    listMock.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    const first = { items: items(0, 8), pagination: { nextCursor: "c1" } };
    const { result } = renderHook(() => useRowPages(params, first, 8, 20));
    act(() => result.current.loadMore());
    act(() => result.current.loadMore());
    expect(listMock).toHaveBeenCalledTimes(1);
    await act(async () => resolve({ items: items(8, 8), pagination: {} }));
    expect(result.current.items).toHaveLength(16);
  });
});
