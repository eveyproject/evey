import React from "react";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

const getItemAsyncMock = vi.fn();
vi.mock("expo-secure-store", () => ({
  getItemAsync: (...args: unknown[]) => getItemAsyncMock(...args),
}));

async function getEveyFetch() {
  const { eveyFetch } = await import("@evey/api");
  return vi.mocked(eveyFetch);
}

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("usePlayOnDevices", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getItemAsyncMock.mockResolvedValue(null);
  });

  it("fetches from /api/v1/devices with GET and returns the device list", async () => {
    const fetch = await getEveyFetch();
    const devices = [
      { id: "dev-1", name: "Living Room TV", type: "tv", connectedAt: "2026-06-04T00:00:00Z" },
      { id: "dev-2", name: "iPhone", type: "mobile", connectedAt: "2026-06-04T00:01:00Z" },
    ];
    fetch.mockResolvedValueOnce({ devices });

    const { usePlayOnDevices } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnDevices(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/devices", method: "GET" }),
    );
    expect(result.current.data).toEqual(devices);
  });

  it("returns [] when the response has no devices field", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({});

    const { usePlayOnDevices } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnDevices(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("excludes this install's own registered device from the list", async () => {
    getItemAsyncMock.mockResolvedValue("dev-self");
    const fetch = await getEveyFetch();
    const devices = [
      { id: "dev-self", name: "This Phone", type: "mobile", connectedAt: "2026-06-04T00:00:00Z" },
      { id: "dev-2", name: "Living Room TV", type: "tv", connectedAt: "2026-06-04T00:01:00Z" },
    ];
    fetch.mockResolvedValueOnce({ devices });

    const { usePlayOnDevices } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnDevices(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await waitFor(() => expect(result.current.data).toEqual([devices[1]]));
  });
});

describe("usePlayOnCommand", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("POSTs the command payload to the device-specific endpoint", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ message: "ok" });

    const { usePlayOnCommand } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnCommand(), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate({
        deviceId: "dev-1",
        command: { type: "loadAndPlay", mediaId: "mi-abc123" },
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: "/api/v1/devices/dev-1/command",
        method: "POST",
        data: { type: "loadAndPlay", mediaId: "mi-abc123" },
      }),
    );
  });

  it("URL-encodes the deviceId in the endpoint", async () => {
    const fetch = await getEveyFetch();
    fetch.mockResolvedValueOnce({ message: "ok" });

    const { usePlayOnCommand } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnCommand(), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate({ deviceId: "dev/with spaces", command: { type: "pause" } });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fetch).toHaveBeenCalledWith(
      expect.objectContaining({
        url: `/api/v1/devices/${encodeURIComponent("dev/with spaces")}/command`,
      }),
    );
  });

  it("surfaces errors when the fetch rejects", async () => {
    const fetch = await getEveyFetch();
    fetch.mockRejectedValueOnce(new Error("device not found"));

    const { usePlayOnCommand } = await import("./usePlayOnDevices");
    const { result } = renderHook(() => usePlayOnCommand(), { wrapper: createWrapper() });

    await act(async () => {
      result.current.mutate({ deviceId: "gone", command: { type: "pause" } });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect((result.current.error as Error).message).toBe("device not found");
  });
});
