import { afterAll, describe, expect, it, vi } from "vitest";

// The television has no browser `online` event and no OS network module: its
// query client must learn a lost server from a request that got no answer,
// and ask again when the box wakes up.
const appState = vi.hoisted(() => ({ listeners: [] as ((s: string) => void)[] }));
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_: string, l: (s: string) => void) => {
      appState.listeners.push(l);
      return { remove: () => undefined };
    },
  },
}));
const ping = vi.hoisted(() => vi.fn(async (_url: string) => false));
vi.mock("./server-config", () => ({
  resolveServerUrl: async () => "http://srv.test",
  pingServer: (url: string) => ping(url),
}));

import { onlineManager } from "@tanstack/react-query";
import { queryClient } from "./query-client";

afterAll(() => {
  queryClient.unmount();
  onlineManager.setOnline(true);
});

describe("tv query-client reachability", () => {
  it("goes offline when a request gets no answer and the server does not either", async () => {
    queryClient.mount();
    await queryClient
      .fetchQuery({ queryKey: ["lost"], queryFn: async () => { throw new Error("Network request failed"); }, retry: false })
      .catch(() => undefined);
    await vi.waitFor(() => expect(ping).toHaveBeenCalledWith("http://srv.test"));
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(false));
  });

  it("asks the server again when the box comes back to the foreground", async () => {
    ping.mockClear();
    ping.mockResolvedValue(true);
    appState.listeners.forEach((l) => l("active"));
    await vi.waitFor(() => expect(ping).toHaveBeenCalled());
    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(true));
  });
});
