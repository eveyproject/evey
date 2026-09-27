import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({ Linking: { openURL: vi.fn() } }));
vi.mock("./useServerUrl", () => ({ useServerUrlNullable: () => "http://srv" }));

import { useAppUpdate } from "./useAppUpdate";

const wrapper = ({ children }: { children: React.ReactNode }) =>
  React.createElement(QueryClientProvider, { client: new QueryClient({ defaultOptions: { queries: { retry: false } } }) }, children);

afterEach(() => vi.unstubAllGlobals());

describe("useAppUpdate", () => {
  it("reads the TV release the server advertises on /healthz", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ json: async () => ({ clients: { tv: { version: "9.9.9", version_code: 999999 } } }) });
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useAppUpdate(), { wrapper });
    await waitFor(() => expect(result.current).toEqual({ version: "9.9.9", versionCode: 999999 }));
    expect(fetchMock).toHaveBeenCalledWith("http://srv/healthz");
  });

  it("offers nothing when the check fails", async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error("offline"));
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useAppUpdate(), { wrapper });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(result.current).toBeNull();
  });
});
