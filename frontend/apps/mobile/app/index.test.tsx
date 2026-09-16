import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

// A returning session walks straight in without probing the server (offline
// on a plane, the downloads are the point); the probe gates only the connect
// flow. "Who is watching" is asked on a cold start only when the household
// could pick wrong, and never offline where the picker leads nowhere.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
}));
vi.mock("expo-router", () => ({
  Redirect: ({ href }: { href: string }) => React.createElement("div", { "data-testid": "redirect" }, href),
}));
const auth = vi.hoisted(() => ({ accessToken: null as string | null, profileId: null as string | null }));
const store = vi.hoisted(() => ({ pick: false, loads: 0 }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: typeof auth) => unknown) => sel(auth),
  loadProfileCache: async () => { store.loads++; return []; },
  shouldPickProfileOnColdStart: () => store.pick,
}));
const server = vi.hoisted(() => ({ url: undefined as string | null | undefined, ok: true, pings: 0, online: true }));
vi.mock("../src/hooks/useServerUrl", () => ({ useServerUrlNullable: () => server.url }));
vi.mock("../src/hooks/useOnlineStatus", () => ({ useOnlineStatus: () => server.online }));
vi.mock("../src/lib/server-config", () => ({ pingServer: async () => { server.pings++; return server.ok; } }));
vi.mock("../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { bg: "#000", accent: "#f00" } }) }));

import EntryRedirect from "./index";

const redirect = () => screen.getByTestId("redirect").textContent;

beforeEach(() => {
  auth.accessToken = "tok";
  auth.profileId = "p1";
  server.url = "https://phone.local";
  server.ok = true;
  server.pings = 0;
  server.online = true;
  store.pick = false;
  store.loads = 0;
});

describe("mobile launch route", () => {
  it("a saved session goes straight in without pinging the server", async () => {
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/(auth)/(tabs)"));
    expect(server.pings).toBe(0);
    expect(store.loads).toBe(1);
  });

  it("asks who is watching on a cold start only when the household could pick wrong, and never offline", async () => {
    store.pick = true;
    const first = render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/profile-select"));
    first.unmount();

    server.online = false;
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/(auth)/(tabs)"));
    expect(store.loads).toBe(1);
  });

  it("without a session the server is probed first: unreachable goes back to connect, reachable to login or the picker", async () => {
    auth.accessToken = null;
    server.ok = false;
    const first = render(<EntryRedirect />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    await waitFor(() => expect(redirect()).toBe("/server-connect"));
    expect(server.pings).toBe(1);
    first.unmount();

    server.ok = true;
    const second = render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/login"));
    second.unmount();

    auth.accessToken = "tok";
    auth.profileId = null;
    render(<EntryRedirect />);
    await waitFor(() => expect(redirect()).toBe("/profile-select"));
  });

  it("waits for the stored URL, then sends a phone with no server to connect", () => {
    server.url = undefined;
    const { rerender } = render(<EntryRedirect />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    server.url = null;
    rerender(<EntryRedirect />);
    expect(redirect()).toBe("/server-connect");
  });
});
