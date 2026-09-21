import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

vi.mock("./server-config", () => ({
  getServerUrl: vi.fn(),
}));

import { getServerUrl } from "./server-config";
import { useServerUrl, useServerUrlNullable } from "./useServerUrl";

const getServerUrlMock = getServerUrl as ReturnType<typeof vi.fn>;

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useServerUrl", () => {
  it("starts with empty string before resolving", async () => {
    getServerUrlMock.mockResolvedValue("http://myserver:9091");
    const { result } = renderHook(() => useServerUrl());
    expect(result.current).toBe("");
    // Flush the pending resolution inside an act() boundary so React doesn't
    // emit a "not wrapped in act(…)" warning when the test exits.
    await act(async () => {});
  });

  it("resolves to server URL", async () => {
    getServerUrlMock.mockResolvedValue("http://myserver:9091");
    const { result } = renderHook(() => useServerUrl());
    await act(async () => {});
    expect(result.current).toBe("http://myserver:9091");
  });

  it("resolves to empty string when getServerUrl returns null", async () => {
    getServerUrlMock.mockResolvedValue(null);
    const { result } = renderHook(() => useServerUrl());
    await act(async () => {});
    expect(result.current).toBe("");
  });
});

describe("useServerUrlNullable", () => {
  it("starts as undefined before resolving", async () => {
    getServerUrlMock.mockResolvedValue("http://srv:8096");
    const { result } = renderHook(() => useServerUrlNullable());
    expect(result.current).toBeUndefined();
    await act(async () => {});
  });

  it("resolves to null when no server configured", async () => {
    getServerUrlMock.mockResolvedValue(null);
    const { result } = renderHook(() => useServerUrlNullable());
    await act(async () => {});
    expect(result.current).toBeNull();
  });

  it("resolves to string when server URL is set", async () => {
    getServerUrlMock.mockResolvedValue("http://myserver:9091");
    const { result } = renderHook(() => useServerUrlNullable());
    await act(async () => {});
    expect(result.current).toBe("http://myserver:9091");
  });
});
