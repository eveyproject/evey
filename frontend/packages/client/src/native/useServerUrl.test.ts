import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const store = vi.hoisted(() => ({ url: "" as string, legacy: null as string | null }));
vi.mock("@evey/store", () => ({
  useServerListStore: (selector: (s: { getActiveServer: () => { url: string } | null }) => unknown) =>
    selector({ getActiveServer: () => (store.url ? { url: store.url } : null) }),
}));
vi.mock("./server-config", () => ({ getServerUrl: async () => store.legacy }));
vi.mock("../log", () => ({ clientLogger: () => ({ warn: () => {} }) }));

import { useServerUrl, useServerUrlNullable } from "./useServerUrl";

describe("useServerUrl", () => {
  beforeEach(() => {
    store.url = "";
    store.legacy = null;
  });

  it("answers the active server on the first render, no keystore round trip", () => {
    store.url = "https://tv.example";
    const { result } = renderHook(() => useServerUrl());
    expect(result.current).toBe("https://tv.example");
  });

  it("falls back to the legacy keystore url only when the store has no server", async () => {
    store.legacy = "https://old.example";
    const { result } = renderHook(() => useServerUrl());
    expect(result.current).toBe("");
    await waitFor(() => expect(result.current).toBe("https://old.example"));
    const nullable = renderHook(() => useServerUrlNullable());
    await waitFor(() => expect(nullable.result.current).toBe("https://old.example"));
  });

  it("the nullable form tells 'no server' (null) apart from 'still reading' (undefined)", async () => {
    const { result } = renderHook(() => useServerUrlNullable());
    expect(result.current).toBeUndefined();
    await waitFor(() => expect(result.current).toBeNull());
  });
});
