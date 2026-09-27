import { describe, it, expect, beforeEach } from "vitest";
import { vi } from "vitest";

vi.mock("./online-manager", () => ({ setupOnlineManager: () => {} }));
const appState = vi.hoisted(() => ({ listeners: [] as ((s: string) => void)[] }));
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_: string, cb: (s: string) => void) => {
      appState.listeners.push(cb);
      return { remove: () => {} };
    },
  },
}));

import { queryClient } from "./query-client";
import { useServerListStore } from "@evey/store";

// The home layout is set per profile on the server; one changed on another
// device must reload when the phone comes back to the foreground.
describe("query cache vs app foreground", () => {
  it("invalidates the home layout when the app becomes active", () => {
    queryClient.setQueryData(["/api/v1/feeds/config", { profileId: "p1" }], { configs: [] });
    appState.listeners.forEach((cb) => cb("active"));
    expect(queryClient.getQueryState(["/api/v1/feeds/config", { profileId: "p1" }])?.isInvalidated).toBe(true);
  });
});

// Query keys do not carry the server: without a clear on switch, server A's
// home rows, media details and profile data are served as server B's until
// each query happens to go stale (5 minutes of wrong library).
describe("query cache vs server switch", () => {
  beforeEach(() => {
    queryClient.clear();
    useServerListStore.setState({ activeServerId: null });
  });

  it("clears the whole cache when the active server changes", () => {
    useServerListStore.setState({ activeServerId: "srv-a" });
    queryClient.setQueryData(["media", "m1"], { title: "from server A" });

    useServerListStore.setState({ activeServerId: "srv-b" });

    expect(queryClient.getQueryData(["media", "m1"])).toBeUndefined();
  });

  it("clears when the last server is removed (active becomes null)", () => {
    useServerListStore.setState({ activeServerId: "srv-a" });
    queryClient.setQueryData(["media", "m1"], { title: "from server A" });

    useServerListStore.setState({ activeServerId: null });

    expect(queryClient.getQueryData(["media", "m1"])).toBeUndefined();
  });

  it("leaves the cache alone on boot-time rehydration (null -> saved id)", () => {
    queryClient.setQueryData(["boot", "warm"], { ok: true });

    useServerListStore.setState({ activeServerId: "srv-a" });

    expect(queryClient.getQueryData(["boot", "warm"])).toEqual({ ok: true });
  });
});
