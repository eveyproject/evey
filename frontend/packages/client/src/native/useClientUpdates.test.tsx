import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";

const servers = vi.hoisted(() => ({
  state: { activeServerId: null as string | null, servers: {} as Record<string, { url: string }> },
}));
vi.mock("@evey/store", () => ({
  useServerListStore: (selector: (s: typeof servers.state) => unknown) => selector(servers.state),
}));

import { useClientUpdates, type ClientUpdateBinding } from "./useClientUpdates";

function binding(): ClientUpdateBinding {
  const data = new Map<string, string>();
  return {
    updates: {
      enabled: true,
      updateId: "update-1",
      isEmbeddedLaunch: false,
      releasedAt: null,
      setUpdateURLAndRequestHeadersOverride: vi.fn(),
      checkForUpdateAsync: vi.fn(async () => ({ isAvailable: false })),
      fetchUpdateAsync: vi.fn(async () => ({ isNew: false })),
    },
    store: { getItem: (key) => data.get(key) ?? null, setItem: (key, value) => void data.set(key, value) },
    app: "tv",
  };
}

beforeEach(() => {
  servers.state = { activeServerId: null, servers: {} };
});

describe("useClientUpdates", () => {
  it("follows the active server", () => {
    servers.state = { activeServerId: "a", servers: { a: { url: "https://a.example" } } };
    const b = binding();
    renderHook(() => useClientUpdates(b, false));

    expect(b.updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenCalledWith({
      updateUrl: "https://a.example/api/v1/client-updates/tv/manifest",
      requestHeaders: {},
    });
  });

  it("stops asking any server when none is active", () => {
    const b = binding();
    renderHook(() => useClientUpdates(b, false));

    expect(b.updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenCalledWith(null);
  });

  it("stops asking a server the user removed", () => {
    servers.state = { activeServerId: "a", servers: { a: { url: "https://a.example" } } };
    const b = binding();
    const { rerender } = renderHook(() => useClientUpdates(b, false));

    servers.state = { activeServerId: null, servers: {} };
    rerender();

    expect(b.updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenLastCalledWith(null);
  });

  it("offers the restart until put off, and again for a later download", () => {
    const b = binding();
    const { result, rerender } = renderHook(({ pending }) => useClientUpdates(b, pending), { initialProps: { pending: true } });
    expect(result.current.offerRestart).toBe(true);

    act(() => result.current.dismiss());
    expect(result.current.offerRestart).toBe(false);

    rerender({ pending: false });
    rerender({ pending: true });
    expect(result.current.offerRestart).toBe(true);
  });
});
