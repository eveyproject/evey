import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const store: Record<string, string> = {};
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn((key: string) => Promise.resolve(store[key] ?? null)),
  setItemAsync: vi.fn((key: string, value: string) => {
    store[key] = value;
    return Promise.resolve();
  }),
  deleteItemAsync: vi.fn((key: string) => {
    delete store[key];
    return Promise.resolve();
  }),
}));

import { useServerUrl, useServerUrlNullable } from "./useServerUrl";

beforeEach(() => {
  Object.keys(store).forEach((k) => delete store[k]);
});

describe("useServerUrl", () => {
  it("returns empty string initially when no URL stored", async () => {
    const { result } = renderHook(() => useServerUrl());
    expect(result.current).toBe("");
    await act(async () => {});
    expect(result.current).toBe("");
  });

  it("returns stored URL after effect runs", async () => {
    store["evey-server-url"] = "http://myserver:8080";
    const { result } = renderHook(() => useServerUrl());
    await act(async () => {});
    expect(result.current).toBe("http://myserver:8080");
  });
});

describe("useServerUrlNullable", () => {
  it("returns undefined initially", async () => {
    const { result } = renderHook(() => useServerUrlNullable());
    expect(result.current).toBeUndefined();
    // The read from secure store resolves after the assertion: flush it
    // inside act rather than letting it update state once the test is over.
    await act(async () => {});
  });

  it("returns null when no URL stored", async () => {
    const { result } = renderHook(() => useServerUrlNullable());
    await act(async () => {});
    expect(result.current).toBeNull();
  });

  it("returns stored URL after effect runs", async () => {
    store["evey-server-url"] = "http://tvserver:8080";
    const { result } = renderHook(() => useServerUrlNullable());
    await act(async () => {});
    expect(result.current).toBe("http://tvserver:8080");
  });
});
