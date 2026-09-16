import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

// The launch route decides where a television lands: no saved server ->
// setup; a saved server that does not answer -> setup with the reason (not a
// blank first-time screen); no session -> login; no profile -> the picker;
// otherwise "who is watching" only when the household could pick wrong.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
}));
vi.mock("expo-router", () => ({
  Redirect: ({ href }: { href: string }) => React.createElement("div", { "data-testid": "redirect" }, href),
}));
const auth = vi.hoisted(() => ({ accessToken: null as string | null, profileId: null as string | null }));
const store = vi.hoisted(() => ({
  cached: [] as unknown[],
  pick: false,
  setServerName: vi.fn(),
  activeServerId: "srv-1" as string | null,
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: typeof auth) => unknown) => sel(auth),
  useServerListStore: { getState: () => ({ activeServerId: store.activeServerId, setServerName: store.setServerName }) },
  loadProfileCache: async () => store.cached,
  shouldPickProfileOnColdStart: () => store.pick,
}));
const server = vi.hoisted(() => ({ url: undefined as string | null | undefined, ok: true, name: "Salon" }));
vi.mock("../src/hooks/useServerUrl", () => ({ useServerUrlNullable: () => server.url }));
vi.mock("../src/lib/server-config", () => ({ probeServerHealth: async () => ({ ok: server.ok, name: server.name }) }));
const seedOffline = vi.hoisted(() => vi.fn());
vi.mock("../src/i18n", () => ({ seedOfflineStrings: () => seedOffline() }));
vi.mock("../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { bg: "#000", accent: "#f00" } }) }));

import EntryRedirect from "./index";

const redirect = () => screen.getByTestId("redirect").textContent;

beforeEach(() => {
  vi.clearAllMocks();
  auth.accessToken = "tok";
  auth.profileId = "p1";
  server.url = "https://tv.local";
  server.ok = true;
  store.pick = false;
  store.activeServerId = "srv-1";
});

describe("tv launch route", () => {
  it("waits for the server list, then sends a box with no server to setup", () => {
    server.url = undefined;
    const { rerender } = render(<EntryRedirect />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    server.url = null;
    rerender(<EntryRedirect />);
    expect(redirect()).toBe("/server-connect");
  });

  it("names an unreachable server as the reason and seeds offline strings", async () => {
    server.ok = false;
    render(<EntryRedirect />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    await waitFor(() => expect(redirect()).toBe("/server-connect?unreachable=1"));
    expect(seedOffline).toHaveBeenCalledTimes(1);
    expect(store.setServerName).not.toHaveBeenCalled();
  });

  it("refreshes the advertised server name, then routes by session and profile", async () => {
    auth.accessToken = null;
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/login"));
    expect(store.setServerName).toHaveBeenCalledWith("srv-1", "Salon");
    expect(seedOffline).not.toHaveBeenCalled();
  });

  it("asks for a profile when the session has none", async () => {
    auth.profileId = null;
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/profile-select"));
  });

  it("lands on the tabs, or on who-is-watching when the household could pick wrong", async () => {
    const first = render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/(auth)/(tabs)"));
    first.unmount();

    store.pick = true;
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/profile-select"));
  });
});
