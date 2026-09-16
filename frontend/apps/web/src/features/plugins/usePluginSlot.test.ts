import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
let authed = true;
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: authed ? "tok" : "" }),
}));

import { usePluginSlotEntries } from "./usePluginSlot";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  eveyFetchMock.mockReset();
  authed = true;
});

// A slot lists the modules of enabled plugins mounted there, with the entry
// path made relative to the plugin's web root (both protojson spellings), the
// manifest name falling back to the id, and the external-fetch allowlist
// carried along for the iframe.
describe("usePluginSlotEntries", () => {
  it("resolves the entries of one slot", async () => {
    eveyFetchMock.mockResolvedValue({
      plugins: [
        { id: "a", name: "Alpha", enabled: true, webRoot: "web", externalFetch: ["api.example.test"], frontend: [{ slot: "settings", src: "web/settings.js" }, { slot: "player", src: "web/player.js" }] },
        { id: "b", enabled: true, web_root: "dist/", external_fetch: [], frontend: [{ slot: "settings", src: "dist/index.js" }] },
        { id: "c", enabled: false, frontend: [{ slot: "settings", src: "x.js" }] },
        { id: "d", enabled: true, webRoot: ".", frontend: [{ slot: "settings", src: "d.js" }] },
        { id: "e", enabled: true, webRoot: "web", frontend: [{ slot: "settings", src: "web" }] },
        { id: "", enabled: true, frontend: [{ slot: "settings", src: "ghost.js" }] },
      ],
    });
    const { result } = renderHook(() => usePluginSlotEntries("settings"), { wrapper: wrapper() });
    await waitFor(() => expect(result.current).toHaveLength(4));
    expect(result.current).toEqual([
      { pluginId: "a", name: "Alpha", entryPath: "settings.js", externalFetch: ["api.example.test"] },
      { pluginId: "b", name: "b", entryPath: "index.js", externalFetch: [] },
      { pluginId: "d", name: "d", entryPath: "d.js", externalFetch: [] },
      { pluginId: "e", name: "e", entryPath: "", externalFetch: [] },
    ]);
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/plugins", method: "GET" });
  });

  it("never asks the server without a session, and an empty answer is an empty slot", async () => {
    authed = false;
    const { result } = renderHook(() => usePluginSlotEntries("settings"), { wrapper: wrapper() });
    expect(result.current).toEqual([]);
    expect(eveyFetchMock).not.toHaveBeenCalled();

    authed = true;
    eveyFetchMock.mockResolvedValue({});
    const second = renderHook(() => usePluginSlotEntries("settings"), { wrapper: wrapper() });
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
    expect(second.result.current).toEqual([]);
  });
});
