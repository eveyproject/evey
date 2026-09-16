import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

let libraries: { id?: string }[] = [];
const listGenresMock = vi.fn();
vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: () => ({ data: { libraries } }),
  libraryServiceListGenres: (...args: unknown[]) => listGenresMock(...args),
}));

import { useAllGenres } from "./useAllGenres";

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  listGenresMock.mockReset();
  libraries = [{ id: "lib-a" }, { id: "lib-b" }, { id: undefined }];
});

// The filter list is what the user's libraries actually contain, fanned out
// per library the caller can see - never a hardcoded English list.
describe("useAllGenres", () => {
  it("stays idle until the caller enables it, and with no library at all", () => {
    const { result } = renderHook(() => useAllGenres(false), { wrapper: wrapper() });
    expect(result.current.fetchStatus).toBe("idle");

    libraries = [];
    const empty = renderHook(() => useAllGenres(true), { wrapper: wrapper() });
    expect(empty.result.current.fetchStatus).toBe("idle");
    expect(listGenresMock).not.toHaveBeenCalled();
  });

  it("asks every readable library once and merges the answers sorted and de-duplicated", async () => {
    listGenresMock.mockImplementation(async (id: string) =>
      id === "lib-a" ? { genres: ["Drame", "Aventure", " "] } : { genres: ["Aventure", "Épouvante"] },
    );
    const { result } = renderHook(() => useAllGenres(true), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(listGenresMock).toHaveBeenCalledTimes(2);
    expect(listGenresMock).toHaveBeenCalledWith("lib-a");
    expect(listGenresMock).toHaveBeenCalledWith("lib-b");
    expect(result.current.data).toEqual(["Aventure", "Drame", "Épouvante"]);
  });

  it("keeps the other libraries' genres when one library answers with an error", async () => {
    listGenresMock.mockImplementation(async (id: string) => {
      if (id === "lib-a") throw new Error("forbidden");
      return { genres: ["Comédie"] };
    });
    const { result } = renderHook(() => useAllGenres(true), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(["Comédie"]);
  });

  it("treats a library without genres as empty rather than failing", async () => {
    listGenresMock.mockResolvedValue({});
    const { result } = renderHook(() => useAllGenres(true), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });
});
