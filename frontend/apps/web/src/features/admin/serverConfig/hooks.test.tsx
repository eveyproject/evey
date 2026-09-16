import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({ set: vi.fn(), del: vi.fn(), listOpts: null as any }));
vi.mock("@evey/api", () => ({
  useServerConfigServiceListServerConfigEntries: (o: any) => {
    api.listOpts = o.query;
    return { data: [] };
  },
  serverConfigServiceSetServerConfigEntry: (...a: unknown[]) => api.set(...a),
  serverConfigServiceDeleteServerConfigEntry: (...a: unknown[]) => api.del(...a),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["/api/v1/admin/server-config"],
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));

import { useServerConfig, useSetConfigEntry, useDeleteConfigEntry, QUERY_KEY } from "./hooks";

let qc: QueryClient;
const wrapper = ({ children }: { children: React.ReactNode }) => React.createElement(QueryClientProvider, { client: qc }, children);

beforeEach(() => {
  vi.clearAllMocks();
  qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  api.set.mockResolvedValue({});
  api.del.mockResolvedValue({});
});

// Reading selects the entries array (never undefined); a write or a delete
// sends the key the server expects, then drops the cached list and confirms;
// a refusal is a toast, not a thrown error the card would have to catch.
describe("server config hooks", () => {
  it("selects the entries array and an empty list when absent", () => {
    renderHook(() => useServerConfig(), { wrapper });
    expect(api.listOpts.select({ entries: [{ key: "a", value: "1" }] })).toEqual([{ key: "a", value: "1" }]);
    expect(api.listOpts.select({})).toEqual([]);
  });

  it("set writes the value, invalidates the list and confirms", async () => {
    const spy = vi.spyOn(qc, "invalidateQueries");
    const { result } = renderHook(() => useSetConfigEntry(), { wrapper });
    result.current.mutate({ key: "server.name", value: "Home" });
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.configUpdated"));
    expect(api.set).toHaveBeenCalledWith("server.name", { value: "Home" });
    expect(spy).toHaveBeenCalledWith({ queryKey: QUERY_KEY });
  });

  it("delete removes the key, invalidates and confirms; refusals toast", async () => {
    const { result } = renderHook(() => useDeleteConfigEntry(), { wrapper });
    result.current.mutate("api.tmdb_key");
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.configDeleted"));
    expect(api.del).toHaveBeenCalledWith("api.tmdb_key");

    api.del.mockRejectedValueOnce(new Error("403"));
    result.current.mutate("api.tmdb_key");
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.serverConfig.failedDeleteEntry"));

    api.set.mockRejectedValueOnce(new Error("403"));
    const set = renderHook(() => useSetConfigEntry(), { wrapper });
    set.result.current.mutate({ key: "k", value: "v" });
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.serverConfig.failedUpdate"));
  });
});
