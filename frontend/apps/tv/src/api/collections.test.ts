import { describe, it, expect, vi, beforeEach } from "vitest";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

import { useMediaItemCollections } from "./collections";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useMediaItemCollections", () => {
  it("fetches the sagas a title belongs to", async () => {
    const data = { collections: [{ id: "c1", name: "Alien", itemCount: 4 }] };
    eveyFetchMock.mockResolvedValue(data);

    const { result } = renderHook(() => useMediaItemCollections("m1"), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/collections",
      method: "GET",
    });
    expect(result.current.data).toEqual(data);
  });

  it("is disabled when mediaItemId is empty", () => {
    const { result } = renderHook(() => useMediaItemCollections(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});
