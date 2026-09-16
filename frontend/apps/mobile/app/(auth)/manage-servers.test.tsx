import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Switching servers probes reachability FIRST (a dead server must not land
// the viewer on a login that cannot work), then rewires the base URL and
// the fetcher and goes to the tabs or to login by whether a token exists;
// the only saved server cannot be deleted; a delete is confirmed by name;
// rename trims; the active row just goes back.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message?: string, buttons?: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, fallback?: string, o?: { name?: string }) => (fallback ?? k).replace("{{name}}", o?.name ?? "") }),
}));
const setBaseURL = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ setBaseURL: (...a: unknown[]) => setBaseURL(...(a as [])) }));
const store = vi.hoisted(() => ({
  servers: {} as Record<string, { id: string; url: string; label?: string; serverName?: string; lastUsedAt: number }>,
  activeServerId: "s1",
  setActiveServer: vi.fn(), removeServer: vi.fn(), renameServer: vi.fn(), setServerName: vi.fn(),
  accessToken: "tok" as string | null,
}));
const toast = vi.hoisted(() => ({ success: vi.fn() }));
vi.mock("@evey/store", () => ({
  useServerListStore: (sel: (s: typeof store) => unknown) => sel(store),
  useAuthStore: { getState: () => ({ accessToken: store.accessToken }) },
  serverDisplayName: (s: { label?: string; serverName?: string; url: string }) => s.label || s.serverName || s.url,
  toast,
}));
const probe = vi.hoisted(() => vi.fn());
const setServerUrl = vi.hoisted(() => vi.fn(async () => {}));
const initFetcher = vi.hoisted(() => vi.fn(async () => {}));
vi.mock("../../src/lib/server-config", () => ({ probeServerHealth: (...a: unknown[]) => probe(...a), setServerUrl: (...a: unknown[]) => setServerUrl(...(a as [])) }));
vi.mock("../../src/lib/fetcher-setup", () => ({ initFetcher: () => initFetcher() }));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", danger: "#f33", success: "#0f0" }, radii: { md: 8, sm: 4 }, spacing: { xs: 4, sm: 8, md: 16, lg: 24 } }),
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
  probe.mockResolvedValue({ probe: "ok", health: { name: "NAS 2" } });
});

describe("manage servers screen (mobile)", () => {
  it("probes before switching, rewires the client, and lands on the tabs with a token or on login without", async () => {
    render(<ManageServersScreen />);
    expect(screen.getByText("Active")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/(auth)/(tabs)"));
    expect(probe).toHaveBeenCalledWith("https://nas");
    expect(store.setServerName).toHaveBeenCalledWith("s2", "NAS 2");
    expect(store.setActiveServer).toHaveBeenCalledWith("s2");
    expect(setBaseURL).toHaveBeenCalledWith("https://nas");
    expect(setServerUrl).toHaveBeenCalledWith("https://nas");
    expect(initFetcher).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("Switched to NAS 2");

    store.accessToken = null;
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));

    fireEvent.click(screen.getByRole("button", { name: "Home" }));
    expect(router.back).toHaveBeenCalledTimes(1);
    expect(probe).toHaveBeenCalledTimes(2);
  });

  it("refuses to switch to a server that does not answer or is not Evey, without touching the client", async () => {
    probe.mockResolvedValueOnce({ probe: "unreachable" });
    render(<ManageServersScreen />);
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(alerts.last?.title).toBe("Connection failed"));
    expect(alerts.last.message).toBe("https://nas did not answer. Check the address or your connection.");
    probe.mockResolvedValueOnce({ probe: "not-evey" });
    fireEvent.click(screen.getByRole("button", { name: "NAS" }));
    await waitFor(() => expect(alerts.last.message).toBe("https://nas answered, but it is not an Evey server."));
    expect(store.setActiveServer).not.toHaveBeenCalled();
    expect(setBaseURL).not.toHaveBeenCalled();
    expect(router.replace).not.toHaveBeenCalled();
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
    fireEvent.click(screen.getByText("Save"));
    expect(store.renameServer).toHaveBeenCalledWith("s1", "Salon");
    expect(screen.queryByTestId("modal")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add another server" }));
    expect(router.push).toHaveBeenCalledWith("/server-connect");
  });

  it("refuses to delete the only saved server", () => {
    store.servers = { s1: store.servers.s1 };
    render(<ManageServersScreen />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(alerts.last.title).toBe("Can't delete the only saved server");
    expect(store.removeServer).not.toHaveBeenCalled();
  });
});
