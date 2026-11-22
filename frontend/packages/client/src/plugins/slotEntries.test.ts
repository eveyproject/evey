import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import {
  PLUGIN_LIST_QUERY_KEY,
  PLUGIN_SETTLING_POLL_MS,
  pluginSettlingInterval,
  refreshPluginLists,
  useViewerListFollowsPluginStates,
} from "./slotEntries";

describe("refreshPluginLists", () => {
  it("reloads the viewer list a home is showing, not only the admin list", async () => {
    const client = new QueryClient();
    const viewer = vi.fn().mockResolvedValue({ plugins: [] });
    const admin = vi.fn().mockResolvedValue({ plugins: [] });
    // The home keeps its list fresh for a minute; the admin list is open beside it.
    const home = new QueryObserver(client, { queryKey: PLUGIN_LIST_QUERY_KEY, queryFn: viewer, staleTime: 60_000 });
    const panel = new QueryObserver(client, { queryKey: ["/api/v1/admin/plugins"], queryFn: admin, staleTime: 60_000 });
    const offHome = home.subscribe(() => {});
    const offPanel = panel.subscribe(() => {});
    await vi.waitFor(() => expect(viewer).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(admin).toHaveBeenCalledTimes(1));

    await refreshPluginLists(client);

    expect(viewer).toHaveBeenCalledTimes(2);
    expect(admin).toHaveBeenCalledTimes(2);
    offHome();
    offPanel();
  });
});

describe("a plugin that is still starting", () => {
  afterEach(() => { vi.useRealTimers(); });

  it("is asked about again until it runs, then the admin list is left alone", async () => {
    vi.useFakeTimers();
    const client = new QueryClient();
    const states = ["PLUGIN_STATE_INITIALIZING", "PLUGIN_STATE_INITIALIZING", "PLUGIN_STATE_RUNNING"];
    const admin = vi.fn(async () => ({ plugins: [{ id: "weather", state: states[Math.min(admin.mock.calls.length - 1, 2)] }] }));
    const panel = new QueryObserver(client, {
      queryKey: ["/api/v1/admin/plugins"],
      queryFn: admin,
      refetchInterval: (query) => pluginSettlingInterval(query.state.data?.plugins),
    });
    const off = panel.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(admin).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(PLUGIN_SETTLING_POLL_MS);
    await vi.advanceTimersByTimeAsync(PLUGIN_SETTLING_POLL_MS);
    expect(admin).toHaveBeenCalledTimes(3);
    expect(panel.getCurrentResult().data?.plugins[0].state).toBe("PLUGIN_STATE_RUNNING");

    await vi.advanceTimersByTimeAsync(PLUGIN_SETTLING_POLL_MS * 5);
    expect(admin).toHaveBeenCalledTimes(3);
    off();
  });

  it("asks nothing again when every plugin is at rest", () => {
    expect(pluginSettlingInterval(undefined)).toBe(false);
    expect(pluginSettlingInterval([{ id: "a", state: "PLUGIN_STATE_RUNNING" }, { id: "b", state: "PLUGIN_STATE_STOPPED" }])).toBe(false);
    expect(pluginSettlingInterval([{ id: "a", state: "PLUGIN_STATE_INITIALIZING" }])).toBe(PLUGIN_SETTLING_POLL_MS);
  });

  it("reloads the viewer list once it has changed state, so its slots appear in the open tab", async () => {
    const client = new QueryClient();
    const viewer = vi.fn().mockResolvedValue({ plugins: [] });
    const home = new QueryObserver(client, { queryKey: PLUGIN_LIST_QUERY_KEY, queryFn: viewer, staleTime: 60_000 });
    const offHome = home.subscribe(() => {});
    await vi.waitFor(() => expect(home.getCurrentResult().isSuccess).toBe(true));
    const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
    type List = Array<{ id: string; state: string }> | undefined;
    const starting: List = [{ id: "weather", state: "PLUGIN_STATE_INITIALIZING" }];

    const hook = renderHook(({ plugins }: { plugins: List }) => useViewerListFollowsPluginStates(plugins), {
      wrapper,
      initialProps: { plugins: undefined as List },
    });
    hook.rerender({ plugins: starting });
    // The same states fetched again are not a change.
    hook.rerender({ plugins: [{ id: "weather", state: "PLUGIN_STATE_INITIALIZING" }] });
    expect(viewer).toHaveBeenCalledTimes(1);

    hook.rerender({ plugins: [{ id: "weather", state: "PLUGIN_STATE_RUNNING" }] });
    await vi.waitFor(() => expect(viewer).toHaveBeenCalledTimes(2));
    offHome();
  });
});
