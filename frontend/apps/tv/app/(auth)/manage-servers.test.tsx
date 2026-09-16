import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Switching rewires the base URL and the fetcher, then lands on the tabs or
// on login by whether a token exists; the active row just goes back; the
// only saved server cannot be deleted; a delete is confirmed by name; rename
// trims; Back takes the remote's initial focus.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel, "data-focus": String(!!hasTVPreferredFocus) }, children),
  Alert: { alert: (title: string, message?: string, buttons?: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s },
}));
const router = vi.hoisted(() => ({ replace: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const setBaseURL = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ setBaseURL: (...a: unknown[]) => setBaseURL(...(a as [])) }));
const store = vi.hoisted(() => ({
  servers: {} as Record<string, { id: string; url: string; label?: string; serverName?: string; lastUsedAt: number }>,
  activeServerId: "s1",
  setActiveServer: vi.fn(), removeServer: vi.fn(), renameServer: vi.fn(),
  accessToken: "tok" as string | null,
}));
vi.mock("@evey/store", () => ({
  useServerListStore: (sel: (s: typeof store) => unknown) => sel(store),
  useAuthStore: { getState: () => ({ accessToken: store.accessToken }) },
  serverDisplayName: (s: { label?: string; serverName?: string; url: string }) => s.label || s.serverName || s.url,
}));
const setServerUrl = vi.hoisted(() => vi.fn(async () => {}));
const initFetcher = vi.hoisted(() => vi.fn(async () => {}));
vi.mock("../../src/lib/server-config", () => ({ setServerUrl: (...a: unknown[]) => setServerUrl(...(a as [])) }));
vi.mock("../../src/lib/fetcher-setup", () => ({ initFetcher: () => initFetcher() }));
vi.mock("../../src/i18n/interpolateFallback", () => ({
  interpolateFallback: (s: string, vars: Record<string, string>) => s.replace(/\{\{(\w+)\}\}/g, (_m, k) => vars[k] ?? _m),
}));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", danger: "#f33", success: "#0f0" }, radii: { md: 8, sm: 4 } }),
}));

import ManageServersScreen from "./manage-servers";

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  store.servers = {
    s1: { id: "s1", url: "https://home", label: "Home", lastUsedAt: 2 },
    s2: { id: "s2", url: "https://nas", serverName: "NAS", lastUsedAt: 1 },
  };
  store.activeServerId = "s1";
  store.accessToken = "tok";
});

describe("tv manage servers screen", () => {
  it("gives Back the initial focus, switches by rewiring the client, and lands on tabs or login by token", async () => {
    render(<ManageServersScreen />);
    expect(screen.getByRole("button", { name: "Back" }).getAttribute("data-focus")).toBe("true");
    expect(screen.getByText("Active")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/(auth)/(tabs)"));
    expect(store.setActiveServer).toHaveBeenCalledWith("s2");
    expect(setBaseURL).toHaveBeenCalledWith("https://nas");
    expect(setServerUrl).toHaveBeenCalledWith("https://nas");
    expect(initFetcher).toHaveBeenCalledTimes(1);

    store.accessToken = null;
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));

    fireEvent.click(screen.getByRole("button", { name: "Home" }));
    expect(router.back).toHaveBeenCalledTimes(1);
    expect(store.setActiveServer).toHaveBeenCalledTimes(2);
  });

  it("confirms a delete by name, refuses to delete the only server, and renames with a trimmed label", () => {
    render(<ManageServersScreen />);
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1]);
    expect(alerts.last.message).toContain("NAS will be removed");
    expect(store.removeServer).not.toHaveBeenCalled();
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(store.removeServer).toHaveBeenCalledWith("s2");

    fireEvent.click(screen.getAllByRole("button", { name: "Rename" })[0]);
    const input = screen.getByPlaceholderText("Server name") as HTMLInputElement;
    expect(input.value).toBe("Home");
    fireEvent.change(input, { target: { value: "  Salon  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(store.renameServer).toHaveBeenCalledWith("s1", "Salon");
    expect(screen.queryByTestId("modal")).toBeNull();
  });

  it("refuses to delete the only saved server", () => {
    store.servers = { s1: store.servers.s1 };
    render(<ManageServersScreen />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(alerts.last.title).toBe("Can't delete the only saved server");
    expect(store.removeServer).not.toHaveBeenCalled();
  });
});
