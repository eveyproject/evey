import { describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { PLUGIN_LIST_QUERY_KEY, refreshPluginLists } from "./slotEntries";

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
